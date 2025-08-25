package nested

//go:generate go run ../../cmd/graftgen -interface=UserMapper,AddressMapper -output=graft_gen.go

type User struct {
	ID   int
	Name string
	Addr Address
}

type Address struct {
	Street string
	City   string
}

type UserDTO struct {
	ID   int
	Name string
	Addr AddressDTO
}

type AddressDTO struct {
	Street string
	City   string
}

type UserMapper interface {
	UserToDTO(User) UserDTO
}

type AddressMapper interface {
	AddressToDTO(Address) AddressDTO
}
