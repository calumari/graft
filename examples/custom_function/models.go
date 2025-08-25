package custom_function

import "errors"

//go:generate go run ../../cmd/graftgen -interface=Mapper -output=graft_gen.go

type A struct {
	N int
}

type B struct {
	N int
}

// // Custom function (no error)
func AToB(a A) B {
	return B{N: a.N}
}

// Custom function (with error)
func AToBErr(a A) (B, error) {
	return B{}, errors.New("something went wrong")
}

type Mapper interface {
	Map(a A) B
	MapErr(a A) (B, error)
}
