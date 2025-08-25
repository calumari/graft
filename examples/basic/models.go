package basic

//go:generate go run ../../cmd/graftgen -interface=UserMapper -output=graft_gen.go

type User struct {
	ID   int
	Name string
}

type UserDTO struct {
	ID   int
	Name string
}

type UserMapper interface {
	UserToDTO(User) UserDTO
}
