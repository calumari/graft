package generator

import (
	"fmt"
	"go/types"
	"strings"
)

// buildMethodModel models a single mapper interface method, constructing its IR nodes.
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
	srcStruct, _ := underlyingStruct(srcType)
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
		if len(params) == 1 { // single param -> delegate to helper
			helperName := g.ensureStructHelper(srcType, destType)
			callExpr := helperName + "(" + primaryName + ")"
			nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: callExpr, WithError: hasError})
		} else { // inline multi-param resolution
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
			for i := 0; i < destStruct.NumFields(); i++ {
				df := destStruct.Field(i)
				if !df.Exported() {
					continue
				}
				fname := df.Name()
				tag := destStruct.Tag(i)
				parsed := parseTag(tag)
				mapsrc := parsed["mapsrc"]
				var srcParamName, srcFieldName string
				if mapsrc != "" { // param.path override
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
					if len(pathParts) > 0 {
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
					if len(pathParts) == 1 {
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
					if !resolved { // full type match attempt
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
	} else if compositeAllowed { // top-level collection
		helperName := g.ensureCompositeHelper(srcType, destType)
		callExpr := helperName + "(" + primaryName + ")"
		nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: callExpr, WithError: hasError})
	}
	destTypeStr := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
	mm := &methodModel{Name: m.Name(), Params: params, PrimaryParam: primaryName, DestType: destTypeStr, HasError: hasError, Body: nodes, HasContext: g.currentCtxName != ""}
	return mm, nil
}
