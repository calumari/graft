package generator

import (
	"go/types"
)

// buildMethodModelFromPlan constructs the methodModel body using an existing
// methodPlan.
func (g *generator) buildMethodModelFromPlan(mp *methodPlan) (*methodModel, error) {
	sig := mp.signature
	params := mp.params
	ctxIndex := mp.ctxIndex
	useCtx := ctxIndex >= 0 && ctxIndex < len(params)

	primaryIdx := mp.primaryIndex
	srcType := sig.Params().At(primaryIdx).Type()
	destType := sig.Results().At(0).Type()
	destStruct, destPtr := underlyingStruct(destType)
	primaryName := params[primaryIdx].Name

	var nodes []codeNode
	var err error

	switch {
	case mp.structMapping:
		nodes, err = g.buildStructMethodNodes(mp, sig, params, ctxIndex, primaryName, srcType, destType, destStruct, destPtr, useCtx)
		if err != nil {
			return nil, err
		}
	case mp.compositeMapping:
		helperName := g.ensureCompositeHelper(srcType, destType)
		callExpr := helperName + "(" + primaryName + ")"
		nodes = []codeNode{{Kind: nodeKindReturn, Expr: callExpr, WithError: mp.hasError}}
	}

	destTypeStr := types.TypeString(destType, g.qualifier)

	mm := &methodModel{
		Name:         mp.name,
		Params:       params,
		PrimaryParam: primaryName,
		DestType:     destTypeStr,
		HasError:     mp.hasError,
		Body:         nodes,
		HasContext:   useCtx,
	}

	return mm, nil
}

// buildStructMethodNodes returns IR nodes for a struct mapping method (single or multi param).
func (g *generator) buildStructMethodNodes(mp *methodPlan, sig *types.Signature, params []paramModel, ctxIndex int, primaryName string, srcType, destType types.Type, destStruct *types.Struct, destPtr, useCtx bool) ([]codeNode, error) {
	// Single-param: delegate directly to helper for clarity.
	if len(params) == 1 {
		helperName := g.ensureStructHelper(srcType, destType)
		callExpr := helperName + "(" + primaryName + ")"
		return []codeNode{{Kind: nodeKindReturn, Expr: callExpr, WithError: mp.hasError}}, nil
	}

	// Multi-param: nil-guard pointer params, init dest, resolve field plans, return.
	var nodes []codeNode
	forGuard := g.collectPtrParamNames(sig, params, ctxIndex)
	for _, pn := range forGuard {
		nodes = append(nodes, codeNode{Kind: nodeKindIfNilReturn, Var: pn, Zero: g.zeroValue(destType), WithError: mp.hasError})
	}

	initVar := "dst"
	if destPtr {
		initVar = "mapped"
		if pt, ok := destType.(*types.Pointer); ok {
			under := types.TypeString(pt.Elem(), g.qualifier)
			nodes = append(nodes, codeNode{Kind: nodeKindDestInitAlloc, Var: initVar, UnderType: under})
		}
	} else {
		// Value destination: simple zero-value var decl.
		nodes = append(nodes, codeNode{Kind: nodeKindDestInit, Var: initVar, DestType: types.TypeString(destType, g.qualifier)})
	}

	plans, err := g.resolver.methodStructPlans(mp, sig, destStruct, destPtr, params, ctxIndex, primaryName, useCtx)
	if err != nil {
		return nil, err
	}
	for _, ap := range plans {
		nodes = append(nodes, ap.Nodes...)
	}

	// Return pointer or value directly (pointer already allocated above).
	nodes = append(nodes, codeNode{Kind: nodeKindReturn, Expr: initVar, WithError: mp.hasError})
	return nodes, nil
}

// collectPtrParamNames returns names of struct params that are pointers (excluding context param).
func (g *generator) collectPtrParamNames(sig *types.Signature, params []paramModel, ctxIndex int) []string {
	var out []string
	for i := 0; i < sig.Params().Len(); i++ {
		if i == ctxIndex {
			continue
		}
		pname := params[i].Name
		if _, isPtr := underlyingStruct(sig.Params().At(i).Type()); isPtr {
			out = append(out, pname)
		}
	}

	return out
}

// populateMethods converts all methodPlans of an interface into concrete
// methodModels.
func (g *generator) populateMethods(im *interfaceModel, plans []*methodPlan) error {
	for _, mp := range plans {
		mm, err := g.buildMethodModelFromPlan(mp)
		if err != nil {
			return err
		}
		im.Methods = append(im.Methods, *mm)
	}

	return nil
}
