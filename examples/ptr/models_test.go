package ptr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPtr(t *testing.T) {
	t.Run("user value input maps to user dto value", func(t *testing.T) {
		m := NewUserMapper()
		in := User{ID: 1, Name: "Alice"}
		out := m.ToDTO(in)
		require.Equal(t, 1, out.ID)
		require.Equal(t, "Alice", out.Name)
	})

	t.Run("nil user pointer input maps to zero user dto value", func(t *testing.T) {
		m := NewUserMapper()
		var in *User
		out := m.ToDTOFromPtr(in)
		require.Zero(t, out)
	})

	t.Run("user pointer input maps to user dto value", func(t *testing.T) {
		m := NewUserMapper()
		in := &User{ID: 1, Name: "Alice"}
		out := m.ToDTOFromPtr(in)
		require.Equal(t, 1, out.ID)
		require.Equal(t, "Alice", out.Name)
	})

	t.Run("nil user pointer input maps to nil user dto pointer", func(t *testing.T) {
		m := NewUserMapper()
		var in *User
		out := m.ToDTOPtr(in)
		require.Nil(t, out)
	})

	t.Run("user pointer input maps to user dto pointer", func(t *testing.T) {
		m := NewUserMapper()
		in := &User{ID: 1, Name: "Alice"}
		out := m.ToDTOPtr(in)
		require.NotNil(t, out)
		require.Equal(t, 1, out.ID)
		require.Equal(t, "Alice", out.Name)
	})

	t.Run("user value input maps to user dto pointer", func(t *testing.T) {
		m := NewUserMapper()
		in := User{ID: 1, Name: "Alice"}
		out := m.ToDTOPtrFromVal(in)
		require.NotNil(t, out)
		require.Equal(t, 1, out.ID)
		require.Equal(t, "Alice", out.Name)
	})

	t.Run("multiple user pointer params map to user dto pointer", func(t *testing.T) {
		m := NewUserMapper()
		in1 := &User{ID: 1, Name: "Alice"}
		in2 := &User{ID: 2, Name: "Bob"}
		out := m.ToDTOFromMultipleUsers(in1, in2)
		require.NotNil(t, out)
		require.Equal(t, 1, out.ID)
		require.Equal(t, "Alice", out.Name)
	})
}
