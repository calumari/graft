package recursive

//go:generate go run ../../cmd/graftgen -interface=RecMapper -output=graft_gen.go

type A struct {
	Text string
	B    *B
}

type B struct {
	Text string
	A    *A
}

type RecMapper interface {
	AToB(A) B
}
