package hybrid_test

import (
	"testing"

	"arca/internal/retrieval/hybrid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamedPolicies(t *testing.T) {
	t.Run("balanced resolves to the M3-compatible default", func(t *testing.T) {
		p, err := hybrid.PolicyByName("balanced")
		require.NoError(t, err)
		assert.Equal(t, hybrid.DefaultFusionPolicy(), p)
	})

	t.Run("densebiased resolves to the calibrated sweep value", func(t *testing.T) {
		p, err := hybrid.PolicyByName("densebiased")
		require.NoError(t, err)
		assert.Equal(t, 1.0, p.DenseWeight)
		assert.Equal(t, 0.5, p.SparseWeight)
		assert.Equal(t, 0, p.SparseCap)
		assert.Equal(t, 60.0, p.RRFK)
	})

	t.Run("unknown policy names are rejected", func(t *testing.T) {
		_, err := hybrid.PolicyByName("lexicalbiased")
		require.Error(t, err)
		_, err = hybrid.PolicyByName("bogus")
		require.Error(t, err)
	})
}
