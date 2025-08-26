package generator

import "strings"

// analyzeHelperErrors consolidates: fixed-point helper error marking, node annotation,
// and success return node adjustment.
func (g *generator) analyzeHelperErrors(interfaces *[]interfaceModel) {
	index := map[string]*helperModel{}
	for i := range g.helperModels {
		index[g.helperModels[i].Name] = &g.helperModels[i]
	}

	var nodeProducesError func(*codeNode) bool
	nodeProducesError = func(n *codeNode) bool {
		switch n.Kind {
		case nodeKindAssignMethod, nodeKindPtrMethodMap:
			if n.WithError {
				return true
			}
		case nodeKindAssignHelper, nodeKindPtrStructMap:
			if h := index[n.Helper]; h != nil && h.HasError {
				return true
			}
		}
		if n.WithError {
			return true
		}
		for i := range n.Children {
			if nodeProducesError(&n.Children[i]) {
				return true
			}
		}
		return false
	}

	changed := true
	for changed {
		changed = false
		for i := range g.helperModels {
			h := &g.helperModels[i]
			if h.HasError {
				continue
			}
			produced := false
			for ni := range h.Body {
				if nodeProducesError(&h.Body[ni]) {
					produced = true
					break
				}
			}
			if produced {
				h.HasError = true
				changed = true
			}
		}
	}

	helperErr := map[string]bool{}
	for i := range g.helperModels {
		if g.helperModels[i].HasError {
			helperErr[g.helperModels[i].Name] = true
		}
	}

	var annotate func(*codeNode)
	annotate = func(n *codeNode) {
		switch n.Kind {
		case nodeKindAssignHelper, nodeKindPtrStructMap:
			if helperErr[n.Helper] {
				n.WithError = true
			}
		case nodeKindReturn:
			if idx := strings.Index(n.Expr, "("); idx > 0 {
				if helperErr[n.Expr[:idx]] {
					n.WithError = false
				}
			}
		}
		for i := range n.Children {
			annotate(&n.Children[i])
		}
	}
	for hi := range g.helperModels {
		for ni := range g.helperModels[hi].Body {
			annotate(&g.helperModels[hi].Body[ni])
		}
	}
	for ii := range *interfaces {
		im := &(*interfaces)[ii]
		for mi := range im.Methods {
			for ni := range im.Methods[mi].Body {
				annotate(&im.Methods[mi].Body[ni])
			}
		}
	}

	for hi := range g.helperModels {
		if g.helperModels[hi].HasError {
			for ni := range g.helperModels[hi].Body {
				if g.helperModels[hi].Body[ni].Kind == nodeKindReturn {
					g.helperModels[hi].Body[ni].WithError = true
				}
			}
		}
	}
}
