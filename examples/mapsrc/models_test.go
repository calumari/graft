package mapsrc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapSrc(t *testing.T) {
	t.Run("nested fields flatten", func(t *testing.T) {
		m := NewBuilder()
		out := m.Build(Input{P: Profile{Name: "bob", Detail: Detail{Code: "C"}}, Label: "L"})
		require.Equal(t, "bob", out.UserName)
		require.Equal(t, "C", out.Code)
		require.Equal(t, "L", out.Label)
	})
}
