package basic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBasic(t *testing.T) {
	t.Run("user input maps to user dto", func(t *testing.T) {
		m := NewUserMapper()
		in := User{ID: 1, Name: "Alice", Addr: Address{Street: "123 Main St", City: "Wonderland"}}
		out := m.UserToDTO(in)
		require.Equal(t, 1, out.ID)
		require.Equal(t, "Alice", out.Name)
		require.Equal(t, "123 Main St", out.Addr.Street)
		require.Equal(t, "Wonderland", out.Addr.City)
	})
}
