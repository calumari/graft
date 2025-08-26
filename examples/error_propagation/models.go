package error_propagation

import "fmt"

//go:generate go run ../../cmd/graftgen -interface=Mapper -output=graft_gen.go

type Item struct {
	V int
}

type ItemDTO struct {
	V int
}

type Input struct {
	Items []Item
}

type Output struct {
	Items []ItemDTO `mapfn:"ItemToDTO"`
}

// Custom function returns error for negative values.
func ItemToDTO(i Item) (ItemDTO, error) {
	if i.V < 0 {
		return ItemDTO{}, fmt.Errorf("negative: %d", i.V)
	}
	return ItemDTO(i), nil
}

type Mapper interface {
	Map(Input) (Output, error)
}
