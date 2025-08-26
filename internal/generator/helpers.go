package generator

import (
	"fmt"
	"go/types"
)

// buildAssignmentNodes maps srcExpr->destExpr with type-driven logic and may
// create helpers.
func (g *generator) buildAssignmentNodes(destExpr, srcExpr string, destType, srcType types.Type, currentMethod string, useCtx bool) []codeNode {
	if types.Identical(destType, srcType) {
		return []codeNode{{Kind: nodeKindAssignDirect, Dest: destExpr, Src: srcExpr}}
	}

	if types.AssignableTo(srcType, destType) {
		return []codeNode{{Kind: nodeKindAssignCast, Dest: destExpr, Src: srcExpr, CastType: types.TypeString(destType, g.qualifier)}}
	}

	if key := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier); true {
		if mi, ok := g.registry[key]; ok && mi.Name != currentMethod {
			if mi.Kind == regKindCustomFunc || currentMethod != "" {
				if mi.Kind == regKindCustomFunc {
					return []codeNode{{Kind: nodeKindAssignFunc, Dest: destExpr, Method: mi.Name, Arg: srcExpr, WithError: mi.HasError, UseContext: useCtx}}
				}
				return []codeNode{{Kind: nodeKindAssignMethod, Dest: destExpr, Method: mi.Name, Arg: srcExpr, WithError: mi.HasError, UseContext: useCtx}}
			}
		}
	}

	switch dt := destType.(type) {
	case *types.Slice:
		if st, ok := srcType.(*types.Slice); ok {
			delem, selem := dt.Elem(), st.Elem()
			child := g.buildAssignmentNodes("mapped", "v", delem, selem, currentMethod, useCtx)
			loopErr := false
			for i := range child {
				if child[i].WithError {
					loopErr = true
					break
				}
			}
			return []codeNode{{Kind: nodeKindSliceMap, Src: srcExpr, Dest: destExpr, DestType: types.TypeString(destType, g.qualifier), ElemType: types.TypeString(delem, g.qualifier), Children: child, LoopWithError: loopErr}}
		}
	case *types.Array:
		if st, ok := srcType.(*types.Array); ok && dt.Len() == st.Len() {
			delem, selem := dt.Elem(), st.Elem()
			child := g.buildAssignmentNodes(fmt.Sprintf("%s[i]", destExpr), fmt.Sprintf("%s[i]", srcExpr), delem, selem, currentMethod, useCtx)
			return []codeNode{{Kind: nodeKindArrayMap, Src: srcExpr, Dest: destExpr, Children: child}}
		}
	case *types.Map:
		if st, ok := srcType.(*types.Map); ok && types.Identical(dt.Key(), st.Key()) {
			dval, sval := dt.Elem(), st.Elem()
			child := g.buildAssignmentNodes("mapped", "v", dval, sval, currentMethod, useCtx)
			loopErr := false
			for i := range child {
				if child[i].WithError {
					loopErr = true
					break
				}
			}
			return []codeNode{{Kind: nodeKindMapMap, Src: srcExpr, Dest: destExpr, DestType: types.TypeString(destType, g.qualifier), ElemType: types.TypeString(dval, g.qualifier), Children: child, LoopWithError: loopErr}}
		}
	case *types.Pointer:
		if st, ok := srcType.(*types.Pointer); ok && isStructLike(dt.Elem()) && isStructLike(st.Elem()) {
			key := types.TypeString(st.Elem(), g.qualifier) + "->" + types.TypeString(dt.Elem(), g.qualifier)
			if mi, ok := g.registry[key]; ok && mi.Name != currentMethod {
				if mi.Kind == regKindCustomFunc || currentMethod != "" {
					kind := nodeKindPtrMethodMap
					if mi.Kind == regKindCustomFunc {
						kind = nodeKindPtrFuncMap
					}
					return []codeNode{{Kind: kind, Src: srcExpr, Dest: destExpr, Method: mi.Name, WithError: mi.HasError, UseContext: useCtx}}
				}
			}
			helper := g.ensureStructHelper(srcType, destType)
			return []codeNode{{Kind: nodeKindPtrStructMap, Src: srcExpr, Dest: destExpr, Helper: helper, UseContext: useCtx}}
		}
	}

	if isStructLike(destType) && isStructLike(srcType) {
		helper := g.ensureStructHelper(srcType, destType)
		return []codeNode{{Kind: nodeKindAssignHelper, Dest: destExpr, Src: srcExpr, Helper: helper, UseContext: useCtx}}
	}

	return []codeNode{{Kind: nodeKindUnsupported, SrcType: srcType.String(), DestType: destType.String()}}
}

func (g *generator) ensureStructHelper(srcType, destType types.Type) string {
	key := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
	if name, ok := g.helperNames[key]; ok {
		return name
	}
	name := g.helperName(srcType, destType, false)
	g.helperNames[key] = name

	sStruct, sPtr := underlyingStruct(srcType)
	_, dPtr := underlyingStruct(destType)
	if sStruct == nil {
		return name
	}
	zeroRet := g.zeroValue(destType)
	underDest := ""
	if dPtr {
		zeroRet = "nil"
		if pt, ok := destType.(*types.Pointer); ok {
			underDest = types.TypeString(pt.Elem(), g.qualifier)
		}
	}
	plan := helperPlan{
		name:               name,
		srcType:            srcType,
		destType:           destType,
		srcIsPtr:           sPtr,
		destIsPtr:          dPtr,
		underDestType:      underDest,
		zeroReturn:         zeroRet,
		customFuncName:     "", // only set if a matching custom function exists; avoid self-recursion
		customFuncHasError: false,
		populated:          false,
		composite:          false,
	}
	baseKey := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
	if mi, ok := g.registry[baseKey+"#err"]; ok && mi.Kind == regKindCustomFunc && mi.HasError {
		plan.customFuncName = mi.Name
		plan.customFuncHasError = true
	}
	if plan.customFuncName == "" {
		if mi, ok := g.registry[baseKey]; ok && mi.Kind == regKindCustomFunc {
			plan.customFuncName = mi.Name
			plan.customFuncHasError = false
		}
	}
	g.helperPlans = append(g.helperPlans, plan)
	return name
}

func (g *generator) ensureCompositeHelper(srcType, destType types.Type) string {
	key := "comp:" + types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
	if name, ok := g.helperNames[key]; ok {
		return name
	}
	name := g.helperName(srcType, destType, true)
	g.helperNames[key] = name
	plan := helperPlan{name: name, srcType: srcType, destType: destType, composite: true}
	g.helperPlans = append(g.helperPlans, plan)
	return name
}

func (g *generator) populateHelpers(scope *types.Scope) {
	for i := 0; i < len(g.helperPlans); i++ {
		plan := g.helperPlans[i]
		if plan.populated {
			continue
		}
		if plan.composite {
			body := g.buildAssignmentNodes("dst", "in", plan.destType, plan.srcType, "", false)
			hm := helperModel{
				Name:          plan.name,
				SrcType:       types.TypeString(plan.srcType, g.qualifier),
				DestType:      types.TypeString(plan.destType, g.qualifier),
				Body:          body,
				HasError:      false,
				HasContext:    false,
				SrcIsPtr:      false,
				DestIsPtr:     false,
				UnderDestType: "",
				ZeroReturn:    "",
			}
			g.helperModels = append(g.helperModels, hm)
			plan.populated = true
			continue
		}
		if plan.customFuncName != "" {
			hm := helperModel{
				Name:          plan.name,
				SrcType:       types.TypeString(plan.srcType, g.qualifier),
				DestType:      types.TypeString(plan.destType, g.qualifier),
				SrcIsPtr:      plan.srcIsPtr,
				DestIsPtr:     plan.destIsPtr,
				UnderDestType: plan.underDestType,
				ZeroReturn:    plan.zeroReturn,
				Body:          []codeNode{{Kind: nodeKindAssignFunc, Dest: "dst", Method: plan.customFuncName, Arg: "in", WithError: plan.customFuncHasError}},
				HasError:      plan.customFuncHasError,
				HasContext:    false,
			}
			g.helperModels = append(g.helperModels, hm)
			plan.populated = true
			continue
		}
		plans := g.resolver.helperStructPlans(plan, scope)
		if plans == nil {
			plan.populated = true
			continue
		}
		var body []codeNode
		for _, ap := range plans {
			body = append(body, ap.Nodes...)
		}
		hasErr := false
		for i := range body {
			if body[i].WithError {
				hasErr = true
				break
			}
		}
		hm := helperModel{
			Name:          plan.name,
			SrcType:       types.TypeString(plan.srcType, g.qualifier),
			DestType:      types.TypeString(plan.destType, g.qualifier),
			SrcIsPtr:      plan.srcIsPtr,
			DestIsPtr:     plan.destIsPtr,
			UnderDestType: plan.underDestType,
			ZeroReturn:    plan.zeroReturn,
			Body:          body,
			HasError:      hasErr,
			HasContext:    false,
		}
		g.helperModels = append(g.helperModels, hm)
		plan.populated = true
	}
}
