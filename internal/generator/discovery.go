package generator

import (
	"fmt"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/packages"
)

// loadDir loads the Go package(s) for a directory.
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

// validateMethodSig enforces signature shape constraints.
func validateMethodSig(m *types.Func, sig *types.Signature) error {
	if sig.Params().Len() < 1 {
		return fmt.Errorf("method %s: must have at least one parameter", m.Name())
	}
	if rl := sig.Results().Len(); rl < 1 || rl > 2 {
		return fmt.Errorf("method %s: must have 1 or 2 results", m.Name())
	}
	if sig.Results().Len() == 2 && !isErrorType(sig.Results().At(1).Type()) {
		return fmt.Errorf("method %s: second result must be error", m.Name())
	}

	return nil
}

// buildParamModels derives param models, context index and primary mapping index.
func (g *generator) buildParamModels(sig *types.Signature) (params []paramModel, ctxIdx, primaryIdx int, err error) {
	ctxIdx = -1
	primaryIdx = -1

	nonCtxCount := 0
	for pi := 0; pi < sig.Params().Len(); pi++ {
		p := sig.Params().At(pi)
		pname := p.Name()
		if nt, ok := p.Type().(*types.Named); ok {
			if obj := nt.Obj(); obj != nil && obj.Name() == "Context" {
				if pkg := obj.Pkg(); pkg != nil && pkg.Path() == "context" {
					ctxIdx = pi
					if pname == "" { // default name
						pname = "ctx"
					}
				}
			}
		}
		if pi != ctxIdx { // non-context param
			if pname == "" {
				pname = "p" + strconv.Itoa(nonCtxCount)
			}
			nonCtxCount++
		}
		params = append(params, paramModel{Name: pname, Type: types.TypeString(p.Type(), g.qualifier)})
		if pi == ctxIdx {
			continue
		}
		if primaryIdx == -1 {
			if s, _ := underlyingStruct(p.Type()); s != nil || isCollectionLike(p.Type()) {
				primaryIdx = pi
			}
		}
	}

	if primaryIdx == -1 {
		return nil, -1, -1, fmt.Errorf("no struct or collection parameter to map from")
	}

	return params, ctxIdx, primaryIdx, nil
}

// buildInterfaceModel constructs the model for a single interface type.
func (g *generator) buildInterfaceModel(name string, iface *types.Interface) (*interfaceModel, []*methodPlan, error) {
	implName := lowerFirst(name) + "Impl"
	im := &interfaceModel{Name: name, ImplName: implName}

	var plans []*methodPlan
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			return nil, nil, fmt.Errorf("method %s: not a signature", m.Name())
		}
		if err := validateMethodSig(m, sig); err != nil {
			return nil, nil, err
		}

		// register interface method (for nested helper references) unless custom func variant present
		srcT := types.TypeString(sig.Params().At(0).Type(), g.qualifier)
		destT := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
		key := srcT + "->" + destT
		if _, ok := g.registry[key]; !ok && g.findCustomVariant(key) == nil {
			g.registry[key] = registryEntry{Name: m.Name(), HasError: sig.Results().Len() == 2, Kind: regKindInterfaceMethod}
		}

		params, ctxIdx, primaryIdx, err := g.buildParamModels(sig)
		if err != nil {
			return nil, nil, fmt.Errorf("method %s: %v", m.Name(), err)
		}

		srcType := sig.Params().At(primaryIdx).Type()
		destType := sig.Results().At(0).Type()

		srcStruct, _ := underlyingStruct(srcType)
		destStruct, dptr := underlyingStruct(destType)

		structMap := srcStruct != nil && destStruct != nil
		composite := isCollectionLike(srcType) && isCollectionLike(destType)

		if !structMap && !composite {
			return nil, nil, fmt.Errorf("method %s: unsupported top-level mapping (%s -> %s)", m.Name(), srcType.String(), destType.String())
		}
		// For simple single-param struct/composite mapping, pre-plan helper shell
		if structMap && len(params) == 1 {
			g.ensureStructHelper(srcType, destType)
		}
		if composite {
			g.ensureCompositeHelper(srcType, destType)
		}
		mp := &methodPlan{
			name:             m.Name(),
			signature:        sig,
			params:           params,
			primaryIndex:     primaryIdx,
			ctxIndex:         ctxIdx,
			hasError:         sig.Results().Len() == 2,
			structMapping:    structMap,
			compositeMapping: composite,
			implName:         implName,
		}
		_ = dptr // currently unused at plan level
		plans = append(plans, mp)
	}

	return im, plans, nil
}

// discoverCustomFuncs finds eligible custom mapping functions.
func (g *generator) discoverCustomFuncs(pkg *packages.Package, allowlist []string) map[string]registryEntry {
	allowed := map[string]bool{}
	if len(allowlist) > 0 {
		for _, n := range allowlist {
			allowed[n] = true
		}
	}

	res := map[string]registryEntry{}
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
		if _, exists := g.registry[srcT+"->"+destT]; exists {
			continue
		}

		res[key] = registryEntry{Name: name, HasError: sig.Results().Len() == 2, Kind: regKindCustomFunc}
	}

	return res
}
