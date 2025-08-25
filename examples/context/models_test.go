package ctxex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContext(t *testing.T) {
	t.Run("context parameter passes through mapping", func(t *testing.T) {
		m := NewCtxMapper()
		out := m.Map(context.Background(), In{V: 42})
		require.Equal(t, 42, out.V)
	})
}
