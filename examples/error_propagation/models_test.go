package error_propagation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorPropagation(t *testing.T) {
	m := NewMapper()

	t.Run("negative item returns error", func(t *testing.T) {
		_, err := m.Map(Input{Items: []Item{{V: -1}}})
		require.Error(t, err)
	})

	t.Run("items map", func(t *testing.T) {
		out, err := m.Map(Input{Items: []Item{{V: 5}}})
		require.NoError(t, err)
		require.Len(t, out.Items, 1)
		require.Equal(t, 5, out.Items[0].V)
	})
}
