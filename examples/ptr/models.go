package ptr

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
	ToDTO(User) UserDTO
	ToDTOFromPtr(*User) UserDTO
	ToDTOPtr(*User) *UserDTO
	ToDTOPtrFromVal(User) *UserDTO
	ToDTOFromMultipleUsers(*User, *User) *UserDTO // toy example of multiple source params (only one is mapped)
}
