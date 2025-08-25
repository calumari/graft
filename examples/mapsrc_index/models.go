package mapsrc_index

import "context"

// Demonstrates mapsrc param index selector (mapsrc:"p2.Field")
//go:generate go run ../../cmd/graftgen -interface=Composer -output=graft_gen.go

type A struct {
	Common string
	ValueA string
}

type B struct {
	Common string
	ValueB string
}

type Out struct {
	FromA  string `mapsrc:"a.ValueA"`
	FromB  string `mapsrc:"b.ValueB"`
	Common string
}

// Index-based variant using p0 / p1 (zero-based) tokens
type OutIdx struct {
	FromA  string `mapsrc:"p0.ValueA"`
	FromB  string `mapsrc:"p1.ValueB"`
	Common string `mapsrc:"p0.Common"`
}

type Composer interface {
	Compose(a A, b B) Out
	ComposeIdx(A, B) OutIdx
	ComposeIdxContext(context.Context /* ignored */, A, B) OutIdx
}
