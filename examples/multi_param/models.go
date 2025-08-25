package multi_param

//go:generate go run ../../cmd/graftgen -interface=Assembler -output=graft_gen.go

type Meta struct {
	Version string
}

type Core struct {
	ID   int
	Name string
}

type Combined struct {
	ID      int
	Name    string
	Version string
}

type Assembler interface {
	Assemble(Core, Meta) Combined
}
