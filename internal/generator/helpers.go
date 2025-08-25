package generator

import (
	"fmt"
	"go/types"
	"strings"
)

// buildAssignmentNodes produces nodes mapping srcExpr (srcType) to destExpr
// (destType). Order of checks matters: identical -> assignable (cast) ->
// existing custom/mapper -> collections/pointers -> helper -> unsupported.
func (g *generator) buildAssignmentNodes(destExpr, srcExpr string, destType, srcType types.Type, currentMethod string) []codeNode {
	if types.Identical(destType, srcType) {
		return []codeNode{{Kind: nodeKindAssignDirect, Dest: destExpr, Src: srcExpr}}
	}
	if types.AssignableTo(srcType, destType) {
		return []codeNode{{Kind: nodeKindAssignCast, Dest: destExpr, Src: srcExpr, CastType: types.TypeString(destType, g.qualifier)}}
	}
	// custom / existing mapper
	if key := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier); true {
		if mi, ok := g.registry[key]; ok && mi.Name != currentMethod {
			if mi.Kind == regKindCustomFunc || currentMethod != "" {
				if mi.Kind == regKindCustomFunc {
					return []codeNode{{Kind: nodeKindAssignFunc, Dest: destExpr, Method: mi.Name, Arg: srcExpr, WithError: mi.HasError, UseContext: g.currentCtxName != "", CtxName: g.currentCtxName}}
				}
				return []codeNode{{Kind: nodeKindAssignMethod, Dest: destExpr, Method: mi.Name, Arg: srcExpr, WithError: mi.HasError, UseContext: g.currentCtxName != "", CtxName: g.currentCtxName}}
			}
		}
	}
	switch dt := destType.(type) {
	case *types.Slice:
		if st, ok := srcType.(*types.Slice); ok {
			delem, selem := dt.Elem(), st.Elem()
			child := g.buildAssignmentNodes("mapped", "v", delem, selem, currentMethod)
			loopErr := false
			for _, c := range child {
				if c.WithError {
					loopErr = true
					break
				}
			}
			return []codeNode{{Kind: nodeKindSliceMap, Src: srcExpr, Dest: destExpr, DestType: types.TypeString(destType, g.qualifier), ElemType: types.TypeString(delem, g.qualifier), Children: child, LoopWithError: loopErr}}
		}
	case *types.Array:
		if st, ok := srcType.(*types.Array); ok && dt.Len() == st.Len() {
			delem, selem := dt.Elem(), st.Elem()
			child := g.buildAssignmentNodes(fmt.Sprintf("%s[i]", destExpr), fmt.Sprintf("%s[i]", srcExpr), delem, selem, currentMethod)
			return []codeNode{{Kind: nodeKindArrayMap, Src: srcExpr, Dest: destExpr, Children: child}}
		}
	case *types.Map:
		if st, ok := srcType.(*types.Map); ok {
			if types.Identical(dt.Key(), st.Key()) {
				dval, sval := dt.Elem(), st.Elem()
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
				key := types.TypeString(st.Elem(), g.qualifier) + "->" + types.TypeString(dt.Elem(), g.qualifier)
				if mi, ok := g.registry[key]; ok && mi.Name != currentMethod {
					if mi.Kind == regKindCustomFunc || currentMethod != "" {
						kind := nodeKindPtrMethodMap
						if mi.Kind == regKindCustomFunc {
							kind = nodeKindPtrFuncMap
						}
						return []codeNode{{Kind: kind, Src: srcExpr, Dest: destExpr, Method: mi.Name, WithError: mi.HasError, UseContext: g.currentCtxName != "", CtxName: g.currentCtxName}}
					}
				}
				helper := g.ensureStructHelper(srcType, destType)
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

// ensureStructHelper creates (or reuses) a helper for struct<-struct mappings.
func (g *generator) ensureStructHelper(srcType, destType types.Type) string {
	key := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
	if g.helperSet[key] {
		return helperNameFor(key)
	}
	g.helperSet[key] = true
	name := helperNameFor(key)
	sStruct, sPtr := underlyingStruct(srcType)
	dStruct, dPtr := underlyingStruct(destType)
	var body []codeNode
	zeroRet := g.zeroValue(destType)
	underDest := ""
	if dPtr {
		zeroRet = "nil"
		if pt, ok := destType.(*types.Pointer); ok {
			underDest = types.TypeString(pt.Elem(), g.qualifier)
		}
	}
	if sStruct != nil && dStruct != nil {
		baseKey := types.TypeString(srcType, g.qualifier) + "->" + types.TypeString(destType, g.qualifier)
		if mi, ok := g.registry[baseKey+"#err"]; ok && mi.Kind == regKindCustomFunc && mi.HasError {
			body = append(body, codeNode{Kind: nodeKindAssignFunc, Dest: "dst", Method: mi.Name, Arg: "in", WithError: true})
			g.helperModels = append(g.helperModels, helperModel{Name: name, SrcType: types.TypeString(srcType, g.qualifier), DestType: types.TypeString(destType, g.qualifier), Body: body, SrcIsPtr: sPtr, DestIsPtr: dPtr, UnderDestType: underDest, ZeroReturn: zeroRet, HasError: true})
			return name
		}
		if mi, ok := g.registry[baseKey]; ok && mi.Kind == regKindCustomFunc {
			body = append(body, codeNode{Kind: nodeKindAssignFunc, Dest: "dst", Method: mi.Name, Arg: "in", WithError: false})
			g.helperModels = append(g.helperModels, helperModel{Name: name, SrcType: types.TypeString(srcType, g.qualifier), DestType: types.TypeString(destType, g.qualifier), Body: body, SrcIsPtr: sPtr, DestIsPtr: dPtr, UnderDestType: underDest, ZeroReturn: zeroRet})
			return name
		}
		destVar := "dst"
		srcPrefix := "in."
		for i := 0; i < dStruct.NumFields(); i++ {
			df := dStruct.Field(i)
			if !df.Exported() {
				continue
			}
			fname := df.Name()
			var explicitFunc, explicitSrcPath string
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
						if sf == nil {
							r := []rune(sourceName)
							if len(r) > 0 {
								r[0] = []rune(strings.ToUpper(string(r[0])))[0]
								sf = findMatchingSourceField(sStruct, string(r))
							}
						}
					}
				}
			}
			if explicitSrcPath != "" && explicitFunc == "" {
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
					body = append(body, g.buildAssignmentNodes(destVar+"."+fname, expr, df.Type(), currType, "")...)
					continue
				}
			}
			if sf == nil && explicitFunc == "" {
				body = append(body, codeNode{Kind: nodeKindComment, Comment: "no source field for " + fname})
				continue
			}
			if explicitFunc != "" {
				if g.pkgScope != nil {
					if obj := g.pkgScope.Lookup(explicitFunc); obj != nil {
						if fn, ok := obj.(*types.Func); ok {
							if sig, ok := fn.Type().(*types.Signature); ok && sig.Params().Len() == 1 && sig.Results().Len() >= 1 {
								if sig.Results().Len() == 1 || (sig.Results().Len() == 2 && isErrorType(sig.Results().At(1).Type())) {
									if dslice, okd := df.Type().(*types.Slice); okd {
										child := []codeNode{{Kind: nodeKindAssignFunc, Dest: "mapped", Method: explicitFunc, Arg: "v", WithError: sig.Results().Len() == 2}}
										loopErr := sig.Results().Len() == 2
										body = append(body, codeNode{Kind: nodeKindSliceMap, Src: "in." + sf.Name(), Dest: destVar + "." + fname, DestType: types.TypeString(dslice, g.qualifier), ElemType: types.TypeString(dslice.Elem(), g.qualifier), Children: child, LoopWithError: loopErr})
										continue
									}
									if dmap, okd := df.Type().(*types.Map); okd {
										child := []codeNode{{Kind: nodeKindAssignFunc, Dest: "mapped", Method: explicitFunc, Arg: "v", WithError: sig.Results().Len() == 2}}
										loopErr := sig.Results().Len() == 2
										body = append(body, codeNode{Kind: nodeKindMapMap, Src: "in." + sf.Name(), Dest: destVar + "." + fname, DestType: types.TypeString(dmap, g.qualifier), ElemType: types.TypeString(dmap.Elem(), g.qualifier), Children: child, LoopWithError: loopErr})
										continue
									}
									body = append(body, codeNode{Kind: nodeKindAssignFunc, Dest: destVar + "." + fname, Method: explicitFunc, Arg: "in." + sf.Name(), WithError: sig.Results().Len() == 2})
									continue
								}
							}
						}
					}
				}
				body = append(body, codeNode{Kind: nodeKindComment, Comment: "mapfn not found or invalid: " + explicitFunc})
				continue
			}
			if sf != nil && explicitFunc == "" {
				body = append(body, g.buildAssignmentNodes(destVar+"."+fname, srcPrefix+sf.Name(), df.Type(), sf.Type(), "")...)
			}
		}
	}
	g.helperModels = append(g.helperModels, helperModel{Name: name, SrcType: types.TypeString(srcType, g.qualifier), DestType: types.TypeString(destType, g.qualifier), Body: body, SrcIsPtr: sPtr, DestIsPtr: dPtr, UnderDestType: underDest, ZeroReturn: zeroRet})
	return name
}

// ensureCompositeHelper creates a helper for top-level slice/array/map mapping.
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
