package recursive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecursive(t *testing.T) {
	t.Run("structs with cycle map without infinite recursion", func(t *testing.T) {
		m := NewRecMapper()
		a := A{Text: "hello"}
		b := B{Text: "world"}
		a.B = &b
		b.A = &a // create cycle, mapper should not recurse infinitely
		out := m.AToB(a)
		require.Equal(t, "hello", out.Text)
		require.Nil(t, out.A) // no source field on B for nested A
	})
}
