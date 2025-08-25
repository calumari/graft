package ctxex

import "context"

//go:generate go run ../../cmd/graftgen -interface=CtxMapper -output=graft_gen.go

type In struct {
	V int
}

type Out struct {
	V int
}

type CtxMapper interface {
	Map(context.Context, In) Out
}
