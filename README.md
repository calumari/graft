# Graft

Tiny code generator for **type-safe struct mappers** in Go (inspired by [MapStruct](https://mapstruct.org/)). You write small interfaces; Graft emits plain Go (no reflection) that does the mapping.

## Install

```bash
go install github.com/calumari/graft@latest
```

Or use a `//go:generate` directive (recommended).

## Quick Start

```go
// model.go
package demo

//go:generate go run github.com/calumari/graft/cmd/graftgen -interface=UserMapper -output=mapper_gen.go

type User struct { ID int; Name string }
type UserDTO struct { ID int; Name string }

type UserMapper interface {
    UserToDTO(User) UserDTO
}
```

Generate & use:

```bash
go generate ./...
```

```go
m := NewUserMapper()
dto := m.UserToDTO(User{ID: 1, Name: "Alice"})
```

## Examples

See the `examples/` directory for focused scenarios covering collections, multiple parameters with `mapsrc` tags, context, custom functions, error propagation, and recursion. Each example contains its own minimal test showing expected behavior.
