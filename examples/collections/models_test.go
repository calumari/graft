package collections

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollections(t *testing.T) {
	m := NewColMapper()

	t.Run("slice maps elements", func(t *testing.T) {
		out, err := m.Map([]Elem{{V: 1}, {V: 2}})
		require.NoError(t, err)
		require.Len(t, out, 2)
		require.Equal(t, 1, out[0].V)
		require.Equal(t, 2, out[1].V)
	})

	t.Run("slice with failing element returns error and short circuits", func(t *testing.T) {
		in := SliceContainer{Items: []Elem{{V: 1}, {V: -2}, {V: 3}}}
		out, err := m.MapSliceContainer(in)
		require.Error(t, err)
		require.Len(t, out.Items, 3)
		require.Equal(t, 1, out.Items[0].V)
		require.Zero(t, out.Items[1].V)
	})

	t.Run("map maps values", func(t *testing.T) {
		in := map[string]Elem{"a": {V: 5}, "b": {V: 6}}
		out, err := m.MapMap(in)
		require.NoError(t, err)
		require.Len(t, out, 2)
		require.Equal(t, 5, out["a"].V)
		require.Equal(t, 6, out["b"].V)
	})

	t.Run("map with failing value returns error", func(t *testing.T) {
		in := MapContainer{Items: map[string]Elem{"bad": {V: -1}}}
		_, err := m.MapMapContainer(in)
		require.Error(t, err)
	})
}
