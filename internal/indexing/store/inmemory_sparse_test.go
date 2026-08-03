package store_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryVectorStore_Sparse(t *testing.T) {
	ctx := context.Background()
	s := store.NewInMemoryVectorStore()

	pt := store.VectorPoint{
		ID:     "pt-1",
		Vector: []float32{1.0, 0.0, 0.0},
		Sparse: &sparse.SparseVector{
			Indices: []uint32{1, 3},
			Values:  []float32{2.0, 4.0},
		},
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-1",
			ContentHash: "hash-1",
		},
	}
	require.NoError(t, s.UpsertPoints(ctx, []store.VectorPoint{pt}))

	t.Run("list preserves the sparse vector", func(t *testing.T) {
		listed, err := s.ListPoints(ctx, indexingmodel.MetadataFilter{DocumentIDs: []string{"doc-1"}})
		require.NoError(t, err)
		require.Len(t, listed, 1)
		require.NotNil(t, listed[0].Sparse)
		assert.Equal(t, []uint32{1, 3}, listed[0].Sparse.Indices)
		assert.Equal(t, []float32{2.0, 4.0}, listed[0].Sparse.Values)
	})

	t.Run("sparse search computes dot product over shared indices", func(t *testing.T) {
		results, err := s.SearchVector(ctx, store.VectorSearchQuery{
			Sparse: &sparse.SparseVector{
				Indices: []uint32{1, 3},
				Values:  []float32{3.0, 5.0},
			},
			TopK: 10,
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		// dot(2*3 + 4*5) = 6 + 20 = 26
		assert.InDelta(t, 26.0, results[0].Score, 1e-6)
		assert.Equal(t, "chk-1", results[0].Metadata.ChunkID)
	})

	t.Run("sparse search applies the min score threshold", func(t *testing.T) {
		results, err := s.SearchVector(ctx, store.VectorSearchQuery{
			Sparse:   &sparse.SparseVector{Indices: []uint32{1}, Values: []float32{3.0}},
			TopK:     10,
			MinScore: 7.0, // dot(2*3)=6 < 7
		})
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}
