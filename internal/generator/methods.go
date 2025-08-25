package generator

import (
	"go/types"
)

// buildMethodModelFromPlan constructs the methodModel body using an existing
// methodPlan.
func (g *generator) buildMethodModelFromPlan(mp methodPlan) (*methodModel, error) {
	useCtx := false
	sig := mp.Signature
	params := mp.Params
	ctxIndex := mp.CtxIndex
	if ctxIndex >= 0 && ctxIndex < len(params) {
		useCtx = true
	}
	hasError := mp.HasError
	primaryIdx := mp.PrimaryIndex
	srcType := sig.Params().At(primaryIdx).Type()
	destType := sig.Results().At(0).Type()
	destStruct, destPtr := underlyingStruct(destType)
	primaryName := params[primaryIdx].Name
	var nodes []codeNode
	if mp.StructMapping {
		if len(params) == 1 { // delegate via helper
			helperName := g.ensureStructHelper(srcType, destType)
			callExpr := helperName + "(" + primaryName + ")"
			nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: callExpr, WithError: hasError})
		} else {
			paramPtrs := map[string]bool{}
			for i := 0; i < sig.Params().Len(); i++ {
				if i == ctxIndex {
					continue
				}
				pname := params[i].Name
				if _, isPtr := underlyingStruct(sig.Params().At(i).Type()); isPtr {
					paramPtrs[pname] = true
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
			plans, err := g.resolver.methodStructPlans(mp, sig, destStruct, destPtr, params, ctxIndex, primaryName, useCtx)
			if err != nil {
				return nil, err
			}
			for _, ap := range plans {
				nodes = append(nodes, ap.Nodes...)
			}
			if destPtr {
				nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: "&mapped", WithError: hasError})
			} else {
				nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: "dst", WithError: hasError})
			}
		}
	} else if mp.CompositeMapping {
		helperName := g.ensureCompositeHelper(srcType, destType)
		callExpr := helperName + "(" + primaryName + ")"
		nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: callExpr, WithError: hasError})
	}
	destTypeStr := types.TypeString(sig.Results().At(0).Type(), g.qualifier)
	mm := &methodModel{Name: mp.Name, Params: params, PrimaryParam: primaryName, DestType: destTypeStr, HasError: hasError, Body: nodes, HasContext: ctxIndex >= 0}
	return mm, nil
}

// populateMethods converts all methodPlans of an interface into concrete
// methodModels.
func (g *generator) populateMethods(im *interfaceModel, plans []methodPlan) error {
	for _, mp := range plans {
		mm, err := g.buildMethodModelFromPlan(mp)
		if err != nil {
			return err
		}
		im.Methods = append(im.Methods, *mm)
	}
	return nil
}
