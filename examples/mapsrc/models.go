package mapsrc

//go:generate go run ../../cmd/graftgen -interface=Builder -output=graft_gen.go

type Profile struct {
	Name   string
	Detail Detail
}

type Detail struct {
	Code string
}

type Input struct {
	P     Profile
	Label string
}

type Output struct {
	UserName string `mapsrc:"P.Name"`
	Code     string `mapsrc:"P.Detail.Code"`
	Label    string
}

type Builder interface {
	Build(Input) Output
}
