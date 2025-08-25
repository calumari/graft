package custom_function

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomFunction(t *testing.T) {
	t.Run("custom function maps value", func(t *testing.T) {
		m := NewMapper()
		out := m.Map(A{N: 7})
		require.Equal(t, 7, out.N)
	})

	t.Run("custom function propagates error", func(t *testing.T) {
		m := NewMapper()
		_, err := m.MapErr(&A{N: -1})
		require.Error(t, err)
	})
}
