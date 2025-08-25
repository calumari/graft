package generator

import (
	"fmt"
	"go/types"
	"strings"
)

// AssignmentPlan represents the planned mapping for a single destination field.
type AssignmentPlan struct {
	DestField string
	Nodes     []codeNode // sequence of nodes implementing this assignment (may be one or many)
}

// fieldResolver encapsulates reusable logic for resolving struct field mappings
// using tags (map, mapsrc, mapfn) and source parameter discovery.
type fieldResolver struct{ g *generator }

// helperStructPlans builds assignment plans for a helper mapping (single src
// struct to dest struct).
func (r *fieldResolver) helperStructPlans(plan *helperPlan, scope *types.Scope) []AssignmentPlan {
	sStruct, _ := underlyingStruct(plan.SrcGoType)
	dStruct, _ := underlyingStruct(plan.DestGoType)
	if sStruct == nil || dStruct == nil {
		return nil
	}
	var plans []AssignmentPlan
	for fi := 0; fi < dStruct.NumFields(); fi++ {
		df := dStruct.Field(fi)
		if !df.Exported() {
			continue
		}
		fname := df.Name()
		explicitFunc := ""
		explicitSrcPath := ""
		if pt := parseTagCached(dStruct, fi); pt != nil {
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
			if pt := parseTagCached(dStruct, fi); pt != nil {
				if sourceName := pt["map"]; sourceName != "" {
					sf = findMatchingSourceField(sStruct, sourceName)
					if sf == nil {
						rRunes := []rune(sourceName)
						if len(rRunes) > 0 {
							rRunes[0] = []rune(strings.ToUpper(string(rRunes[0])))[0]
							sf = findMatchingSourceField(sStruct, string(rRunes))
						}
					}
				}
			}
		}
		if explicitSrcPath != "" && explicitFunc == "" {
			parts := strings.Split(explicitSrcPath, ".")
			currType := plan.SrcGoType
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
				nodes := r.g.buildAssignmentNodes("dst."+fname, expr, df.Type(), currType, "", false)
				plans = append(plans, AssignmentPlan{DestField: fname, Nodes: nodes})
				continue
			}
		}
		if sf == nil && explicitFunc == "" {
			plans = append(plans, AssignmentPlan{DestField: fname, Nodes: []codeNode{{Kind: nodeKindComment, Comment: "no source field for " + fname}}})
			continue
		}
		if explicitFunc != "" { // resolve mapfn now (scope available in populateHelpers)
			resolved := false
			if scope != nil {
				if obj := scope.Lookup(explicitFunc); obj != nil {
					if fn, ok := obj.(*types.Func); ok {
						if sig, ok := fn.Type().(*types.Signature); ok && sig.Params().Len() == 1 && sig.Results().Len() >= 1 {
							if sig.Results().Len() == 1 || (sig.Results().Len() == 2 && isErrorType(sig.Results().At(1).Type())) {
								withErr := sig.Results().Len() == 2
								dt := df.Type()
								if dslice, okd := dt.(*types.Slice); okd {
									child := []codeNode{{Kind: nodeKindAssignFunc, Dest: "mapped", Method: explicitFunc, Arg: "v", WithError: withErr}}
									plans = append(plans, AssignmentPlan{DestField: fname, Nodes: []codeNode{{Kind: nodeKindSliceMap, Src: "in." + sf.Name(), Dest: "dst." + fname, DestType: types.TypeString(dslice, r.g.qualifier), ElemType: types.TypeString(dslice.Elem(), r.g.qualifier), Children: child, LoopWithError: withErr}}})
									resolved = true
								} else if dmap, okd := dt.(*types.Map); okd {
									child := []codeNode{{Kind: nodeKindAssignFunc, Dest: "mapped", Method: explicitFunc, Arg: "v", WithError: withErr}}
									plans = append(plans, AssignmentPlan{DestField: fname, Nodes: []codeNode{{Kind: nodeKindMapMap, Src: "in." + sf.Name(), Dest: "dst." + fname, DestType: types.TypeString(dmap, r.g.qualifier), ElemType: types.TypeString(dmap.Elem(), r.g.qualifier), Children: child, LoopWithError: withErr}}})
									resolved = true
								} else {
									srcExpr := "in." + sf.Name()
									nodes := []codeNode{{Kind: nodeKindAssignFunc, Dest: "dst." + fname, Method: explicitFunc, Arg: srcExpr, WithError: withErr}}
									plans = append(plans, AssignmentPlan{DestField: fname, Nodes: nodes})
									resolved = true
								}
							}
						}
					}
				}
			}
			if !resolved {
				plans = append(plans, AssignmentPlan{DestField: fname, Nodes: []codeNode{{Kind: nodeKindComment, Comment: "mapfn not found or invalid: " + explicitFunc}}})
			}
			continue
		}
		if sf != nil {
			nodes := r.g.buildAssignmentNodes("dst."+fname, "in."+sf.Name(), df.Type(), sf.Type(), "", false)
			plans = append(plans, AssignmentPlan{DestField: fname, Nodes: nodes})
		}
	}
	return plans
}

// methodStructPlans resolves field assignments for a multi-param struct mapping
// method. It encapsulates the prior inline logic for mapsrc handling and
// fallback heuristics.
func (r *fieldResolver) methodStructPlans(mp methodPlan, sig *types.Signature, destStruct *types.Struct, destPtr bool, params []paramModel, ctxIndex int, primaryName string, useCtx bool) ([]AssignmentPlan, error) {
	var plans []AssignmentPlan
	// build param struct lookup
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
		if mapsrc != "" {
			parts := strings.Split(mapsrc, ".")
			paramToken := parts[0]
			if strings.HasPrefix(paramToken, "p") {
				idxStr := strings.TrimPrefix(paramToken, "p")
				if idxStr != "" {
					nonCtx := []paramModel{}
					for pi, p := range params {
						if pi == ctxIndex {
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
						nodes := r.g.buildAssignmentNodes(prefixDest(destPtr)+fname, expr, df.Type(), currType, mp.Name, useCtx)
						plans = append(plans, AssignmentPlan{DestField: fname, Nodes: nodes})
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
			plans = append(plans, AssignmentPlan{DestField: fname, Nodes: []codeNode{{Kind: nodeKindComment, Comment: "no struct param for " + fname}}})
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
			// attempt other params
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
						nodes := r.g.buildAssignmentNodes(prefixDest(destPtr)+fname, prefixSrc(p.Name, paramPtrs[p.Name])+f2.Name(), df.Type(), f2.Type(), mp.Name, useCtx)
						plans = append(plans, AssignmentPlan{DestField: fname, Nodes: nodes})
						resolved = true
						break
					}
				}
				if resolved {
					break
				}
			}
			if !resolved {
				for idx, p := range params {
					if idx == ctxIndex {
						continue
					}
					pt := sig.Params().At(idx).Type()
					if types.Identical(pt, df.Type()) {
						plans = append(plans, AssignmentPlan{DestField: fname, Nodes: []codeNode{{Kind: nodeKindAssignDirect, Dest: prefixDest(destPtr) + fname, Src: p.Name}}})
						resolved = true
						break
					}
				}
			}
			if !resolved {
				plans = append(plans, AssignmentPlan{DestField: fname, Nodes: []codeNode{{Kind: nodeKindComment, Comment: "no source for " + fname}}})
			}
			continue
		}
		nodes := r.g.buildAssignmentNodes(prefixDest(destPtr)+fname, prefixSrc(srcParamName, paramPtrs[srcParamName])+sf.Name(), df.Type(), sf.Type(), mp.Name, useCtx)
		plans = append(plans, AssignmentPlan{DestField: fname, Nodes: nodes})
	}
	return plans, nil
}
