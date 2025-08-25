package mapsrc_index

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapSrcIndex(t *testing.T) {
	m := NewComposer()

	t.Run("named params compose into struct", func(t *testing.T) {
		out := m.Compose(A{Common: "c", ValueA: "A"}, B{Common: "c2", ValueB: "B"})
		require.Equal(t, "A", out.FromA)
		require.Equal(t, "B", out.FromB)
		require.Equal(t, "c", out.Common)
	})

	t.Run("indexed params compose into struct", func(t *testing.T) {
		out := m.ComposeIdx(A{Common: "c", ValueA: "X"}, B{Common: "ignored", ValueB: "Y"})
		require.Equal(t, "X", out.FromA)
		require.Equal(t, "Y", out.FromB)
		require.Equal(t, "c", out.Common)
	})

	t.Run("indexed params with context compose into struct", func(t *testing.T) {
		out := m.ComposeIdxContext(t.Context(), A{Common: "c", ValueA: "X"}, B{Common: "ignored", ValueB: "Y"})
		require.Equal(t, "X", out.FromA)
		require.Equal(t, "Y", out.FromB)
		require.Equal(t, "c", out.Common)
	})
}
