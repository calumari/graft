package generator

import "strings"

// computeHelperErrors performs a fixed-point analysis over helper bodies to mark which helpers ultimately return an error.
func (g *generator) computeHelperErrors() map[string]bool {
	changed := true

	index := map[string]*helperModel{}
	for i := range g.helperModels {
		index[g.helperModels[i].Name] = &g.helperModels[i]
	}

	var matchesErrNode func(*codeNode) bool
	matchesErrNode = func(n *codeNode) bool {
		if n.Kind == nodeKindAssignMethod || n.Kind == nodeKindPtrMethodMap {
			return n.WithError
		}
		if n.Kind == nodeKindAssignHelper || n.Kind == nodeKindPtrStructMap {
			if index[n.Helper] != nil && index[n.Helper].HasError {
				return true
			}
		}
		if n.WithError {
			return true
		}
		for i := range n.Children {
			if matchesErrNode(&n.Children[i]) {
				return true
			}
		}
		return false
	}

	marksHelper := func(h *helperModel) bool {
		for i := range h.Body {
			if matchesErrNode(&h.Body[i]) {
				return true
			}
		}
		return false
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

	res := map[string]bool{}
	for i := range g.helperModels {
		if g.helperModels[i].HasError {
			res[g.helperModels[i].Name] = true
		}
	}

	return res
}

// annotateHelperErrorUsage propagates helper error knowledge into node.WithError flags where needed.
func (g *generator) annotateHelperErrorUsage(interfaces *[]interfaceModel, helperErr map[string]bool) {
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
		if n.Expr != "" && helperErr != nil {
			if idx := strings.Index(n.Expr, "("); idx > 0 {
				name := n.Expr[:idx]
				if helperErr[name] {
					n.WithError = false
				}
			}
		}
	}

	for i := range n.Children {
		g.annotateNode(&n.Children[i], helperErr)
	}
}
