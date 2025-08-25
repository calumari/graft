package generator

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// node kinds for template-driven code emission
const (
	nodeKindComment      = "comment"
	nodeKindIfNilReturn  = "ifNilReturn"
	nodeKindDestInit     = "destInit"
	nodeKindAssignDirect = "assignDirect"
	nodeKindAssignCast   = "assignCast"
	nodeKindAssignHelper = "assignHelper"
	nodeKindAssignMethod = "assignMethod"
	nodeKindAssignFunc   = "assignFunc"
	nodeKindSliceMap     = "sliceMap"
	nodeKindArrayMap     = "arrayMap"
	nodeKindMapMap       = "mapMap"
	nodeKindPtrStructMap = "ptrStructMap"
	nodeKindPtrMethodMap = "ptrMethodMap"
	nodeKindPtrFuncMap   = "ptrFuncMap"
	nodeKindReturn       = "return"
	nodeKindUnsupported  = "unsupported"
)

// Config holds generation settings for the mapper generator.
type Config struct {
	Dir         string   // directory to load ("." relative to where command invoked)
	Interfaces  []string // interface type names to implement
	Output      string   // output filename
	CustomFuncs []string // optional: specific custom function names to consider (empty = discover all exported)
	Debug       bool     // when true, inject template debug comments linking nodes to templates
}

// fileModel is the root template model for a generated file.
type fileModel struct {
	Package     string
	Source      string
	Helpers     []helperModel
	Interfaces  []interfaceModel
	NeedContext bool
	Debug       bool
}

// interfaceModel describes a single interface mapping plan.
type interfaceModel struct {
	Name     string
	ImplName string
	Methods  []methodModel
}

// methodModel captures a single interface method mapping plan. PrimaryParam is
// the param chosen as the source root (first struct / collection encountered
// ignoring context param).
type methodModel struct {
	Name         string
	Params       []paramModel
	PrimaryParam string
	DestType     string
	HasError     bool
	Body         []codeNode
	HasContext   bool
}

// paramModel is a lightweight view of a method parameter for templates.
type paramModel struct {
	Name string
	Type string
}

// helperModel represents an internal helper function we synthesize for struct
// or collection mapping.
type helperModel struct {
	Name       string
	SrcType    string
	DestType   string
	Body       []codeNode
	HasError   bool
	HasContext bool
}

// codeNode is an ir node used by templates to emit code fragments.
type codeNode struct {
	Kind          string
	Dest          string
	Src           string
	CastType      string
	Helper        string
	Method        string
	Arg           string
	Comment       string
	Var           string
	Zero          string
	WithError     bool
	Code          string
	DestType      string
	ElemType      string
	SrcType       string
	Expr          string
	Children      []codeNode
	LoopWithError bool
	SuppressNil   bool
	UseContext    bool
	CtxName       string
	// debug fields
	Debug bool
	Path  string
}

// generator holds transient state while building models.
type generator struct {
	currentPkgName string
	methodMap      map[string]methodInfo // key: src->dest
	helperSet      map[string]bool
	helperModels   []helperModel
	helperErrors   map[string]bool // helper name -> returns error
	pkgScope       *types.Scope
	currentCtxName string                // set while building a method if ctx parameter present
	customFuncs    map[string]methodInfo // key: src->dest or src->dest#err for error variant
}

type methodInfo struct {
	Name     string
	HasError bool
	IsFunc   bool
}

// Run executes mapper generation with the provided configuration.
func Run(cfg Config) error { return newGenerator().run(cfg) }

func newGenerator() *generator {
	return &generator{methodMap: make(map[string]methodInfo), helperSet: make(map[string]bool), helperErrors: make(map[string]bool)}
}

func (g *generator) run(cfg Config) error {
	if len(cfg.Interfaces) == 0 {
		return errors.New("no interfaces provided")
	}

	absDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return err
	}

	pkgs, err := loadDir(absDir)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found in %s", absDir)
	}
	// use first package (should only be one for a directory)
	pkg := pkgs[0]

	// record current package name for qualifier logic
	g.currentPkgName = pkg.Name

	ifaceMap := map[string]*types.Interface{}
	missing := []string{}
	scope := pkg.Types.Scope()
	for _, name := range cfg.Interfaces {
		obj := scope.Lookup(name)
		if obj == nil {
			missing = append(missing, name)
			continue
		}
		t, ok := obj.Type().Underlying().(*types.Interface)
		if !ok {
			return fmt.Errorf("%s is not an interface", name)
		}
		ifaceMap[name] = t
	}
	if len(missing) > 0 {
		return fmt.Errorf("interfaces not found: %s", strings.Join(missing, ", "))
	}

	// keep stable ordering
	sort.Strings(cfg.Interfaces)

	// reset helper state (already initialized in newGenerator)
	g.helperSet = make(map[string]bool)
	g.helperModels = nil

	// discover custom funcs; if cfg.CustomFuncs empty discover all exported
	// (see Config doc)
	funcMap := g.discoverCustomFuncs(pkg, cfg.CustomFuncs)
	if g.customFuncs == nil {
		g.customFuncs = make(map[string]methodInfo)
	}
	for k, mi := range funcMap {
		g.customFuncs[k] = mi
		// insert non-error variant into methodMap for nested usage (avoid error
		// variant in non-error contexts)
		if !mi.HasError {
			base := strings.TrimSuffix(k, "#err")
			if _, ok := g.methodMap[base]; !ok {
				g.methodMap[base] = mi
			}
		}
	}
	g.pkgScope = pkg.Types.Scope()

	// build interface models
	var interfaceModels []interfaceModel
	for _, name := range cfg.Interfaces {
		model, err := g.buildInterfaceModel(name, ifaceMap[name])
		if err != nil {
			return err
		}
		interfaceModels = append(interfaceModels, *model)
	}

	// compute helper error status then annotate usage
	g.computeHelperErrors()
	g.annotateHelperErrorUsage(&interfaceModels)

	needCtx := false
	for _, im := range interfaceModels {
		for _, mm := range im.Methods {
			if mm.HasContext {
				needCtx = true
				break
			}
		}
	}
	// propagate context requirement to all helpers if any method uses context
	if needCtx {
		for i := range g.helperModels {
			g.helperModels[i].HasContext = true
		}
	}
	// if debug enabled assign stable path ids for traceable output
	if cfg.Debug {
		var assignPaths func(prefix string, nodes []codeNode)
		assignPaths = func(prefix string, nodes []codeNode) {
			for i := range nodes {
				nodes[i].Debug = true
				nodes[i].Path = prefix + fmt.Sprintf(".%d", i)
				if len(nodes[i].Children) > 0 {
					assignPaths(nodes[i].Path, nodes[i].Children)
				}
			}
		}
		// helpers
		for hi := range g.helperModels {
			assignPaths(fmt.Sprintf("H%d", hi), g.helperModels[hi].Body)
		}
		// interface methods
		for ii := range interfaceModels {
			im := &interfaceModels[ii]
			for mi := range im.Methods {
				assignPaths(fmt.Sprintf("I%d.M%d", ii, mi), im.Methods[mi].Body)
			}
		}
	}

	data := fileModel{Package: pkg.Name, Source: strings.Join(cfg.Interfaces, ", "), Helpers: g.helperModels, Interfaces: interfaceModels, NeedContext: needCtx, Debug: cfg.Debug}

	var out bytes.Buffer
	if err := fileTmpl.ExecuteTemplate(&out, tmplFile, data); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil { // fall back to raw output so user can inspect
		formatted = out.Bytes()
	}

	outPath := filepath.Join(absDir, cfg.Output)
	if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
		return err
	}
	return nil
}

func loadDir(dir string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, "./")
	if err != nil {
		return nil, err
	}
	var result []*packages.Package
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, p.Errors[0]
		}
		result = append(result, p)
	}
	return result, nil
}

func (g *generator) buildInterfaceModel(name string, iface *types.Interface) (*interfaceModel, error) {
	implName := lowerFirst(name) + "Impl"
	im := &interfaceModel{Name: name, ImplName: implName}
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		if sig, ok := m.Type().(*types.Signature); ok && sig.Params().Len() >= 1 && sig.Results().Len() >= 1 {
			primaryIdx := 0
			srcT := types.TypeString(sig.Params().At(primaryIdx).Type(), g.qualifier)
			destT := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
			key := srcT + "->" + destT
			// skip adding interface method mapping if ANY custom variant exists
			_, nonErr := g.customFuncs[key]
			_, errVar := g.customFuncs[key+"#err"]
			if !nonErr && !errVar {
				g.methodMap[key] = methodInfo{Name: m.Name(), HasError: sig.Results().Len() == 2, IsFunc: false}
			}
		}
		mm, err := g.buildMethodModel(implName, m)
		if err != nil {
			return nil, fmt.Errorf("method %s: %w", m.Name(), err)
		}
		im.Methods = append(im.Methods, *mm)
	}
	return im, nil
}

func (g *generator) buildMethodModel(implName string, m *types.Func) (*methodModel, error) {
	g.currentCtxName = ""
	sig, ok := m.Type().(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("not a signature")
	}
	if sig.Params().Len() < 1 {
		return nil, fmt.Errorf("method must have at least one parameter")
	}
	if sig.Results().Len() < 1 || sig.Results().Len() > 2 {
		return nil, fmt.Errorf("method must have 1 or 2 results")
	}
	if sig.Results().Len() == 2 && !isErrorType(sig.Results().At(1).Type()) {
		return nil, fmt.Errorf("second result must be error")
	}

	var params []paramModel
	primaryIdx := -1
	ctxIndex := -1
	for i := 0; i < sig.Params().Len(); i++ {
		p := sig.Params().At(i)
		pname := p.Name()
		if pname == "" {
			pname = fmt.Sprintf("p%d", i)
		}
		if nt, ok := p.Type().(*types.Named); ok {
			if obj := nt.Obj(); obj != nil && obj.Name() == "Context" {
				if pkg := obj.Pkg(); pkg != nil && pkg.Path() == "context" {
					ctxIndex = i
					g.currentCtxName = pname
				}
			}
		}
		params = append(params, paramModel{Name: pname, Type: types.TypeString(p.Type(), g.qualifier)})
		if i == ctxIndex {
			continue
		}
		if primaryIdx == -1 {
			if s, _ := underlyingStruct(p.Type()); s != nil || isCollectionLike(p.Type()) {
				primaryIdx = i
			}
		}
	}
	if primaryIdx == -1 {
		return nil, fmt.Errorf("no struct or collection parameter to map from")
	}

	srcType := sig.Params().At(primaryIdx).Type()
	destType := sig.Results().At(0).Type()
	srcStruct, srcPtr := underlyingStruct(srcType)
	destStruct, destPtr := underlyingStruct(destType)
	isStructMapping := srcStruct != nil && destStruct != nil
	compositeAllowed := isCollectionLike(srcType) && isCollectionLike(destType)
	if !isStructMapping && !compositeAllowed {
		return nil, fmt.Errorf("unsupported top-level mapping (%s -> %s)", srcType.String(), destType.String())
	}
	hasError := sig.Results().Len() == 2

	primaryName := params[primaryIdx].Name
	var nodes []codeNode

	if isStructMapping {
		if len(params) == 1 { // single param path
			baseKey := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
			var mi methodInfo
			var okFn bool
			if hasError {
				mi, okFn = g.customFuncs[baseKey+"#err"]
			} else {
				mi, okFn = g.customFuncs[baseKey]
			}
			if okFn && mi.IsFunc { // direct custom function
				if srcPtr {
					zero := g.zeroValue(destType)
					nodes = append(nodes, codeNode{Kind: nodeKindIfNilReturn, Var: primaryName, Zero: zero, WithError: hasError})
				}
				callArg := primaryName
				if srcPtr {
					callArg = "*" + primaryName
				}
				varName := "dst"
				if destPtr {
					varName = "mapped"
				}
				nodes = append(nodes, codeNode{Kind: nodeKindDestInit, Var: varName, DestType: types.TypeString(destType, g.qualifier)})
				nodes = append(nodes, codeNode{Kind: nodeKindAssignFunc, Dest: varName, Method: mi.Name, Arg: callArg, WithError: mi.HasError})
				retExpr := varName
				if destPtr {
					retExpr = "&" + varName
				}
				nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: retExpr, WithError: hasError})
			} else { // helper path
				var helperName string
				if srcPtr {
					helperName = g.ensureStructHelper(srcStruct, destStruct)
				} else {
					helperName = g.ensureStructHelper(srcType, destType)
				}
				if srcPtr {
					zero := g.zeroValue(destType)
					nodes = append(nodes, codeNode{Kind: nodeKindIfNilReturn, Var: primaryName, Zero: zero, WithError: hasError})
				}
				callArg := primaryName
				if srcPtr {
					callArg = "*" + primaryName
				}
				callExpr := helperName + "(" + callArg + ")"
				if destPtr {
					nodes = append(nodes, codeNode{Kind: nodeKindDestInit, Var: "mapped", DestType: types.TypeString(destType, g.qualifier)})
					nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: "&mapped", WithError: hasError})
				} else {
					nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: callExpr, WithError: hasError})
				}
			}
		} else { // multi-param inline mapping path: inline mapping logic; helper would not simplify
			// prepare param struct map
			paramStructs := map[string]*types.Struct{}
			paramPtrs := map[string]bool{}
			for i := 0; i < sig.Params().Len(); i++ {
				if i == ctxIndex {
					continue
				}
				pname := params[i].Name
				if s, isPtr := underlyingStruct(sig.Params().At(i).Type()); s != nil {
					paramStructs[pname] = s
					paramPtrs[pname] = isPtr
				}
			}
			for pn, isPtr := range paramPtrs {
				if isPtr {
					nodes = append(nodes, codeNode{Kind: nodeKindIfNilReturn, Var: pn, Zero: g.zeroValue(destType), WithError: hasError})
				}
			}
			if destPtr {
				nodes = append(nodes, codeNode{Kind: nodeKindDestInit, Var: "mapped", DestType: types.TypeString(destType, g.qualifier)})
			} else {
				nodes = append(nodes, codeNode{Kind: nodeKindDestInit, Var: "dst", DestType: types.TypeString(destType, g.qualifier)})
			}
			for i := 0; i < destStruct.NumFields(); i++ { // resolve each dest field against params (mapsrc tag overrides)
				df := destStruct.Field(i)
				if !df.Exported() {
					continue
				}
				fname := df.Name()
				tag := destStruct.Tag(i)
				parsed := parseTag(tag)
				mapsrc := parsed["mapsrc"]
				var srcParamName, srcFieldName string
				if mapsrc != "" { // explicit param.path override
					parts := strings.Split(mapsrc, ".")
					paramToken := parts[0]
					if strings.HasPrefix(paramToken, "p") {
						idxStr := strings.TrimPrefix(paramToken, "p")
						if idxStr != "" {
							nonCtx := []paramModel{}
							for i, p := range params {
								if i == ctxIndex {
									continue
								}
								nonCtx = append(nonCtx, p)
							}
							var idx int
							if _, err := fmt.Sscanf(idxStr, "%d", &idx); err == nil {
								if idx >= 0 && idx < len(nonCtx) {
									srcParamName = nonCtx[idx].Name
								}
							}
						}
					}
					if srcParamName == "" {
						for _, p := range params {
							if p.Name == paramToken {
								srcParamName = p.Name
								break
							}
						}
					}
					pathParts := []string{}
					if len(parts) > 1 {
						pathParts = parts[1:]
					}
					if len(pathParts) > 0 { // attempt nested walk to build expression
						paramIdx := -1
						for pi := 0; pi < sig.Params().Len(); pi++ {
							if params[pi].Name == srcParamName {
								paramIdx = pi
								break
							}
						}
						if paramIdx >= 0 {
							currType := sig.Params().At(paramIdx).Type()
							expr := srcParamName
							okPath := true
							for _, seg := range pathParts {
								s, _ := underlyingStruct(currType)
								if s == nil {
									okPath = false
									break
								}
								var f *types.Var
								for fj := 0; fj < s.NumFields(); fj++ {
									ff := s.Field(fj)
									if ff.Exported() && ff.Name() == seg {
										f = ff
										break
									}
								}
								if f == nil {
									okPath = false
									break
								}
								expr += "." + seg
								currType = f.Type()
							}
							if okPath {
								assign := g.buildAssignmentNodes(prefixDest(destPtr)+fname, expr, df.Type(), currType, m.Name())
								nodes = append(nodes, assign...)
								continue
							}
						}
					}
					if len(pathParts) == 1 { // single segment path -> treat as field override
						srcFieldName = pathParts[0]
					}
				}
				if srcParamName == "" {
					srcParamName = primaryName
				}
				if srcFieldName == "" {
					srcFieldName = fname
				}
				sStruct := paramStructs[srcParamName]
				if sStruct == nil {
					nodes = append(nodes, codeNode{Kind: nodeKindComment, Comment: "no struct param for " + fname})
					continue
				}
				var sf *types.Var
				for j := 0; j < sStruct.NumFields(); j++ {
					f := sStruct.Field(j)
					if f.Exported() && f.Name() == srcFieldName {
						sf = f
						break
					}
				}
				if sf == nil {
					resolved := false
					for _, p := range params {
						if p.Name == srcParamName {
							continue
						}
						ss := paramStructs[p.Name]
						if ss == nil {
							continue
						}
						for jj := 0; jj < ss.NumFields(); jj++ {
							f2 := ss.Field(jj)
							if f2.Exported() && f2.Name() == fname {
								assign := g.buildAssignmentNodes(prefixDest(destPtr)+fname, prefixSrc(p.Name, paramPtrs[p.Name])+f2.Name(), df.Type(), f2.Type(), m.Name())
								nodes = append(nodes, assign...)
								resolved = true
								break
							}
						}
						if resolved {
							break
						}
					}
					if !resolved { // final attempt: any param whose full type matches dest field type
						for idx, p := range params {
							if idx == ctxIndex {
								continue
							}
							pt := sig.Params().At(idx).Type()
							if types.Identical(pt, df.Type()) {
								nodes = append(nodes, codeNode{Kind: nodeKindAssignDirect, Dest: prefixDest(destPtr) + fname, Src: p.Name})
								resolved = true
								break
							}
						}
					}
					if !resolved {
						nodes = append(nodes, codeNode{Kind: nodeKindComment, Comment: "no source for " + fname})
					}
					continue
				}
				assign := g.buildAssignmentNodes(prefixDest(destPtr)+fname, prefixSrc(srcParamName, paramPtrs[srcParamName])+sf.Name(), df.Type(), sf.Type(), m.Name())
				nodes = append(nodes, assign...)
			}
			if destPtr {
				nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: "&mapped", WithError: hasError})
			} else {
				nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: "dst", WithError: hasError})
			}
		}
	} else if compositeAllowed {
		// always route through a helper for top-level collection mappings
		helperName := g.ensureCompositeHelper(srcType, destType)
		callExpr := helperName + "(" + primaryName + ")"
		nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: callExpr, WithError: hasError})
	}

	destTypeStr := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
	mm := &methodModel{Name: m.Name(), Params: params, PrimaryParam: primaryName, DestType: destTypeStr, HasError: hasError, Body: nodes, HasContext: g.currentCtxName != ""}
	return mm, nil
}

// composite mapping helpers

// buildAssignmentNodes produces nodes mapping srcExpr (srcType) to destExpr
// (destType). order of checks matters: identical -> assignable (cast) ->
// existing custom/mapper -> collections/pointers -> helper -> unsupported.
func (g *generator) buildAssignmentNodes(destExpr, srcExpr string, destType, srcType types.Type, currentMethod string) []codeNode {
	if types.Identical(destType, srcType) {
		return []codeNode{{Kind: nodeKindAssignDirect, Dest: destExpr, Src: srcExpr}}
	}
	if types.AssignableTo(srcType, destType) {
		return []codeNode{{Kind: nodeKindAssignCast, Dest: destExpr, Src: srcExpr, CastType: types.TypeString(destType, g.qualifier)}}
	}
	// check for user-defined mapper method or allowed custom function first
	{
		key := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
		if mi, ok := g.methodMap[key]; ok && mi.Name != currentMethod {
			// functions allowed anywhere; methods only top-level (currentMethod
			// != "")
			if mi.IsFunc || currentMethod != "" {
				if mi.IsFunc {
					return []codeNode{{Kind: nodeKindAssignFunc, Dest: destExpr, Method: mi.Name, Arg: srcExpr, WithError: mi.HasError, UseContext: g.currentCtxName != "", CtxName: g.currentCtxName}}
				}
				return []codeNode{{Kind: nodeKindAssignMethod, Dest: destExpr, Method: mi.Name, Arg: srcExpr, WithError: mi.HasError, UseContext: g.currentCtxName != "", CtxName: g.currentCtxName}}
			}
		}
	}
	switch dt := destType.(type) {
	case *types.Slice:
		if st, ok := srcType.(*types.Slice); ok {
			delem := dt.Elem()
			selem := st.Elem()
			child := g.buildAssignmentNodes("mapped", "v", delem, selem, currentMethod)
			loopErr := false
			for _, c := range child {
				if c.WithError {
					loopErr = true
					break
				}
			}
			// suppress outer destInit when destExpr is 'dst' to avoid duplicate
			// (helper template initializes)
			return []codeNode{{Kind: nodeKindSliceMap, Src: srcExpr, Dest: destExpr, DestType: types.TypeString(destType, g.qualifier), ElemType: types.TypeString(delem, g.qualifier), Children: child, LoopWithError: loopErr}}
		}
	case *types.Array:
		if st, ok := srcType.(*types.Array); ok && dt.Len() == st.Len() {
			delem := dt.Elem()
			selem := st.Elem()
			child := g.buildAssignmentNodes(fmt.Sprintf("%s[i]", destExpr), fmt.Sprintf("%s[i]", srcExpr), delem, selem, currentMethod)
			return []codeNode{{Kind: nodeKindArrayMap, Src: srcExpr, Dest: destExpr, Children: child}}
		}
	case *types.Map:
		if st, ok := srcType.(*types.Map); ok {
			if types.Identical(dt.Key(), st.Key()) {
				dval := dt.Elem()
				sval := st.Elem()
				child := g.buildAssignmentNodes("mapped", "v", dval, sval, currentMethod)
				loopErr := false
				for _, c := range child {
					if c.WithError {
						loopErr = true
						break
					}
				}
				return []codeNode{{Kind: nodeKindMapMap, Src: srcExpr, Dest: destExpr, DestType: types.TypeString(destType, g.qualifier), ElemType: types.TypeString(dval, g.qualifier), Children: child, LoopWithError: loopErr}}
			}
		}
	case *types.Pointer:
		if st, ok := srcType.(*types.Pointer); ok {
			if isStructLike(dt.Elem()) && isStructLike(st.Elem()) {
				// method mapping for pointer elements if available
				key := types.TypeString(st.Elem(), g.qualifier) + "->" + types.TypeString(dt.Elem(), g.qualifier)
				if mi, ok := g.methodMap[key]; ok && mi.Name != currentMethod {
					if mi.IsFunc || currentMethod != "" { // functions allowed always
						kind := nodeKindPtrMethodMap
						if mi.IsFunc {
							kind = nodeKindPtrFuncMap
						}
						return []codeNode{{Kind: kind, Src: srcExpr, Dest: destExpr, Method: mi.Name, WithError: mi.HasError, UseContext: g.currentCtxName != "", CtxName: g.currentCtxName}}
					}
				}
				helper := g.ensureStructHelper(st.Elem(), dt.Elem())
				return []codeNode{{Kind: nodeKindPtrStructMap, Src: srcExpr, Dest: destExpr, Helper: helper, UseContext: g.currentCtxName != ""}}
			}
		}
	}
	if isStructLike(destType) && isStructLike(srcType) {
		helper := g.ensureStructHelper(srcType, destType)
		return []codeNode{{Kind: nodeKindAssignHelper, Dest: destExpr, Src: srcExpr, Helper: helper, UseContext: g.currentCtxName != ""}}
	}
	return []codeNode{{Kind: nodeKindUnsupported, SrcType: srcType.String(), DestType: destType.String()}}
}

func (g *generator) ensureStructHelper(srcType, destType types.Type) string {
	key := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
	if g.helperSet[key] {
		return helperNameFor(key)
	}
	g.helperSet[key] = true
	name := helperNameFor(key)
	sStruct, _ := underlyingStruct(srcType)
	dStruct, _ := underlyingStruct(destType)
	var body []codeNode
	if sStruct != nil && dStruct != nil {
		for i := 0; i < dStruct.NumFields(); i++ {
			df := dStruct.Field(i)
			if !df.Exported() {
				continue
			}
			fname := df.Name()
			var explicitFunc string
			var explicitSrcPath string
			if tag := dStruct.Tag(i); tag != "" {
				pt := parseTag(tag)
				if fn := pt["mapfn"]; fn != "" {
					explicitFunc = fn
				}
				if p := pt["mapsrc"]; p != "" {
					explicitSrcPath = p
				}
			}
			sf := findMatchingSourceField(sStruct, fname)
			if sf == nil {
				sf = findTaggedSourceField(sStruct, fname)
			}
			if sf == nil {
				if tag := dStruct.Tag(i); tag != "" {
					if sourceName := parseTag(tag)["map"]; sourceName != "" {
						sf = findMatchingSourceField(sStruct, sourceName)
						if sf == nil { // attempt capitalization fallback (emailAddress -> EmailAddress)
							r := []rune(sourceName)
							if len(r) > 0 {
								r[0] = []rune(strings.ToUpper(string(r[0])))[0]
								sf = findMatchingSourceField(sStruct, string(r))
							}
						}
					}
				}
			}
			if explicitSrcPath != "" && explicitFunc == "" { // resolve nested path starting at receiver param 'in'
				parts := strings.Split(explicitSrcPath, ".")
				currType := srcType
				expr := "in"
				okPath := true
				for _, seg := range parts {
					s, _ := underlyingStruct(currType)
					if s == nil {
						okPath = false
						break
					}
					var f *types.Var
					for fj := 0; fj < s.NumFields(); fj++ {
						ff := s.Field(fj)
						if ff.Exported() && ff.Name() == seg {
							f = ff
							break
						}
					}
					if f == nil {
						okPath = false
						break
					}
					expr += "." + seg
					currType = f.Type()
				}
				if okPath {
					body = append(body, g.buildAssignmentNodes("dst."+fname, expr, df.Type(), currType, "")...)
					continue
				}
			}
			if sf == nil && explicitFunc == "" { // no matching field or tag
				body = append(body, codeNode{Kind: nodeKindComment, Comment: "no source field for " + fname})
				continue
			}
			if explicitFunc != "" { // attempt lookup of user provided mapping function
				if g.pkgScope != nil {
					if obj := g.pkgScope.Lookup(explicitFunc); obj != nil {
						if fn, ok := obj.(*types.Func); ok {
							if sig, ok := fn.Type().(*types.Signature); ok && sig.Params().Len() == 1 && sig.Results().Len() >= 1 {
								if sig.Results().Len() == 1 || (sig.Results().Len() == 2 && isErrorType(sig.Results().At(1).Type())) {
									// support slice element mapping
									if dslice, okd := df.Type().(*types.Slice); okd {
										child := []codeNode{{Kind: nodeKindAssignFunc, Dest: "mapped", Method: explicitFunc, Arg: "v", WithError: sig.Results().Len() == 2}}
										loopErr := sig.Results().Len() == 2
										body = append(body, codeNode{Kind: nodeKindSliceMap, Src: "in." + sf.Name(), Dest: "dst." + fname, DestType: types.TypeString(dslice, g.qualifier), ElemType: types.TypeString(dslice.Elem(), g.qualifier), Children: child, LoopWithError: loopErr})
										continue
									}
									// map: per-value
									if dmap, okd := df.Type().(*types.Map); okd {
										child := []codeNode{{Kind: nodeKindAssignFunc, Dest: "mapped", Method: explicitFunc, Arg: "v", WithError: sig.Results().Len() == 2}}
										loopErr := sig.Results().Len() == 2
										body = append(body, codeNode{Kind: nodeKindMapMap, Src: "in." + sf.Name(), Dest: "dst." + fname, DestType: types.TypeString(dmap, g.qualifier), ElemType: types.TypeString(dmap.Elem(), g.qualifier), Children: child, LoopWithError: loopErr})
										continue
									}
									// fallback: simple value mapping
									body = append(body, codeNode{Kind: nodeKindAssignFunc, Dest: "dst." + fname, Method: explicitFunc, Arg: "in." + sf.Name(), WithError: sig.Results().Len() == 2})
									continue
								}
							}
						}
					}
				} // end lookup chain; fallback comment if function not found
				body = append(body, codeNode{Kind: nodeKindComment, Comment: "mapfn not found or invalid: " + explicitFunc})
				continue
			}
			body = append(body, g.buildAssignmentNodes("dst."+fname, "in."+sf.Name(), df.Type(), sf.Type(), "")...)
		}
	}
	g.helperModels = append(g.helperModels, helperModel{Name: name, SrcType: types.TypeString(srcType, g.qualifier), DestType: types.TypeString(destType, g.qualifier), Body: body})
	return name
}

// ensureCompositeHelper creates a helper for top-level slice/array/map mapping
// so interface methods stay thin.
func (g *generator) ensureCompositeHelper(srcType, destType types.Type) string {
	key := "comp:" + types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
	if g.helperSet[key] {
		return helperNameFor(key)
	}
	g.helperSet[key] = true
	name := helperNameFor(key)
	body := g.buildAssignmentNodes("dst", "in", destType, srcType, "")
	g.helperModels = append(g.helperModels, helperModel{Name: name, SrcType: types.TypeString(srcType, g.qualifier), DestType: types.TypeString(destType, g.qualifier), Body: body})
	return name
}

func (g *generator) computeHelperErrors() {
	changed := true
	index := map[string]*helperModel{}
	for i := range g.helperModels {
		index[g.helperModels[i].Name] = &g.helperModels[i]
	}
	marksHelper := func(h *helperModel) bool {
		var needs bool
		for _, n := range h.Body {
			if n.Kind == nodeKindAssignMethod || n.Kind == nodeKindPtrMethodMap {
				if n.WithError {
					needs = true
					break
				}
			}
			if n.Kind == nodeKindAssignHelper || n.Kind == nodeKindPtrStructMap {
				if index[n.Helper] != nil && index[n.Helper].HasError {
					needs = true
					break
				}
			}
			for _, c := range n.Children {
				if c.WithError {
					needs = true
					break
				}
			}
		}
		return needs
	}
	for changed {
		changed = false
		for i := range g.helperModels {
			h := &g.helperModels[i]
			if !h.HasError && marksHelper(h) {
				h.HasError = true
				changed = true
			}
		}
	}
	for _, h := range g.helperModels {
		if h.HasError {
			g.helperErrors[h.Name] = true
		}
	}
}

func (g *generator) annotateHelperErrorUsage(interfaces *[]interfaceModel) {
	helperErr := g.helperErrors
	for hi := range g.helperModels {
		h := &g.helperModels[hi]
		for ni := range h.Body {
			g.annotateNode(&h.Body[ni], helperErr)
		}
	}
	for ii := range *interfaces {
		im := &(*interfaces)[ii]
		for mi := range im.Methods {
			m := &im.Methods[mi]
			for ni := range m.Body {
				g.annotateNode(&m.Body[ni], helperErr)
			}
		}
	}
}

func (g *generator) annotateNode(n *codeNode, helperErr map[string]bool) {
	switch n.Kind {
	case nodeKindAssignHelper, nodeKindPtrStructMap:
		if helperErr[n.Helper] {
			n.WithError = true
		}
	case nodeKindReturn:
		if n.Expr != "" {
			if idx := strings.Index(n.Expr, "("); idx > 0 {
				if helperErr[n.Expr[:idx]] {
					n.SuppressNil = true
				}
			}
		}
	}
	for i := range n.Children {
		g.annotateNode(&n.Children[i], helperErr)
	}
}

func (g *generator) qualifier(p *types.Package) string {
	if p == nil || p.Name() == g.currentPkgName {
		return ""
	}
	return p.Name()
}
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToLower(string(r[0])))[0]
	return string(r)
}
func isErrorType(t types.Type) bool {
	if named, ok := t.(*types.Named); ok {
		if named.Obj().Pkg() == nil && named.Obj().Name() == "error" {
			return true
		}
	}
	return false
}
func underlyingStruct(t types.Type) (*types.Struct, bool) {
	switch tt := t.(type) {
	case *types.Pointer:
		if s, ok := tt.Elem().Underlying().(*types.Struct); ok {
			return s, true
		}
	default:
		if s, ok := tt.Underlying().(*types.Struct); ok {
			return s, false
		}
	}
	return nil, false
}
func (g *generator) zeroValue(t types.Type) string {
	if _, ok := t.(*types.Pointer); ok {
		return "nil"
	}
	return types.TypeString(t, g.qualifier) + "{}"
}
func isStructLike(t types.Type) bool {
	_, ok := t.Underlying().(*types.Struct)
	return ok
}
func isCollectionLike(t types.Type) bool {
	switch t.(type) {
	case *types.Slice, *types.Array, *types.Map:
		return true
	}
	return false
}
func prefixDest(destPtr bool) string {
	if destPtr {
		return "mapped."
	}
	return "dst."
}
func prefixSrc(param string, isPtr bool) string {
	if isPtr {
		return "*" + param + "."
	}
	return param + "."
}
func helperNameFor(key string) string {
	h := sha1.Sum([]byte(key))
	return "map_" + hex.EncodeToString(h[:6])
}
func findMatchingSourceField(src *types.Struct, name string) *types.Var {
	for i := 0; i < src.NumFields(); i++ {
		f := src.Field(i)
		if f.Exported() && f.Name() == name {
			return f
		}
	}
	return nil
}
func findTaggedSourceField(src *types.Struct, destName string) *types.Var {
	for i := 0; i < src.NumFields(); i++ {
		f := src.Field(i)
		if !f.Exported() {
			continue
		}
		if tag := src.Tag(i); tag != "" {
			parsed := parseTag(tag)
			if v, ok := parsed["map"]; ok && strings.EqualFold(v, destName) {
				return f
			}
		}
	}
	return nil
}
func parseTag(tag string) map[string]string {
	res := map[string]string{}
	tag = strings.Trim(tag, "`")
	for _, p := range strings.Split(tag, " ") {
		p = strings.TrimSpace(p)
		if p == "" || !strings.Contains(p, ":") {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		k := kv[0]
		v := strings.Trim(kv[1], "\"")
		v = strings.Trim(v, "\"")
		res[k] = v
	}
	return res
}
func customFuncKey(src, dest string, hasErr bool) string {
	if hasErr {
		return src + "->" + dest + "#err"
	}
	return src + "->" + dest
}
func (g *generator) discoverCustomFuncs(pkg *packages.Package, allowlist []string) map[string]methodInfo {
	allowed := map[string]bool{}
	if len(allowlist) > 0 {
		for _, n := range allowlist {
			allowed[n] = true
		}
	}
	res := map[string]methodInfo{}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}
		if len(allowlist) > 0 && !allowed[name] {
			continue
		}
		obj := scope.Lookup(name)
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Params().Len() != 1 || sig.Results().Len() < 1 || sig.Results().Len() > 2 {
			continue
		}
		if sig.Results().Len() == 2 && !isErrorType(sig.Results().At(1).Type()) {
			continue
		}
		srcT := types.TypeString(sig.Params().At(0).Type(), g.qualifier)
		destT := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
		key := customFuncKey(srcT, destT, sig.Results().Len() == 2)
		if _, exists := g.methodMap[srcT+"->"+destT]; exists {
			continue
		}
		res[key] = methodInfo{Name: name, HasError: sig.Results().Len() == 2, IsFunc: true}
	}
	return res
}
