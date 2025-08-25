package multi_param

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiParam(t *testing.T) {
	t.Run("core and meta inputs assemble into composite struct", func(t *testing.T) {
		m := NewAssembler()
		out := m.Assemble(Core{ID: 3, Name: "X"}, Meta{Version: "1"})
		require.Equal(t, 3, out.ID)
		require.Equal(t, "X", out.Name)
		require.Equal(t, "1", out.Version)
	})
}
