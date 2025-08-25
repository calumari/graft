package collections

import "fmt"

//go:generate go run ../../cmd/graftgen -interface=ColMapper -output=graft_gen.go

type Elem struct {
	V int
}

type ElemDTO struct {
	V int
}

// Standalone function (not used directly now) plus interface method below to demonstrate error short-circuit.
func ElemToElemDTO(e Elem) (ElemDTO, error) {
	if e.V < 0 {
		return ElemDTO{}, fmt.Errorf("bad")
	}
	return ElemDTO{V: e.V}, nil
}

// Containers to demonstrate mapfn-driven element mapping with error short-circuit
type SliceContainer struct {
	Items []Elem
}
type SliceContainerDTO struct {
	Items []ElemDTO `mapfn:"ElemToElemDTO"`
}
type MapContainer struct {
	Items map[string]Elem
}
type MapContainerDTO struct {
	Items map[string]ElemDTO `mapfn:"ElemToElemDTO"`
}

type ColMapper interface {
	Map([]Elem) ([]ElemDTO, error)
	MapMap(map[string]Elem) (map[string]ElemDTO, error)
	MapSliceContainer(SliceContainer) (SliceContainerDTO, error)
	MapMapContainer(MapContainer) (MapContainerDTO, error)
}
