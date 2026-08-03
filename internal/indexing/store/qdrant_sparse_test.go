package store_test

import (
	"context"
	"testing"

	"arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQdrantVectorStore_Sparse(t *testing.T) {
	ctx := context.Background()
	collections := &fakeCollectionsServer{}
	qs := newTestQdrantStore(t, nil, collections)
	store.WithSparseVectors()(qs)

	require.NoError(t, qs.UpsertPoints(ctx, []store.VectorPoint{{
		ID:     "pt-1",
		Vector: []float32{1.0, 0.0, 0.0},
		Sparse: &sparse.SparseVector{
			Indices: []uint32{1, 3},
			Values:  []float32{2.0, 4.0},
		},
		Metadata: model.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-1",
			ContentHash: "hash-1",
		},
	}}))

	t.Run("collection is created with the named sparse vector config", func(t *testing.T) {
		require.NotNil(t, collections.createReq)
		sparseCfg := collections.createReq.GetSparseVectorsConfig()
		require.NotNil(t, sparseCfg, "expected sparse vectors config")
		_, ok := sparseCfg.GetMap()[store.SparseVectorName]
		assert.True(t, ok, "expected sparse vector named %q in config", store.SparseVectorName)
	})

	t.Run("list restores the sparse vector", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{DocumentIDs: []string{"doc-1"}})
		require.NoError(t, err)
		require.Len(t, listed, 1)
		require.NotNil(t, listed[0].Sparse)
		assert.Equal(t, []uint32{1, 3}, listed[0].Sparse.Indices)
		assert.Equal(t, []float32{2.0, 4.0}, listed[0].Sparse.Values)
	})

	t.Run("sparse search scores by dot product via the query API", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Sparse: &sparse.SparseVector{
				Indices: []uint32{1, 3},
				Values:  []float32{3.0, 5.0},
			},
			TopK: 10,
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		// dot(2*3 + 4*5) = 26
		assert.InDelta(t, 26.0, results[0].Score, 1e-6)
		assert.Equal(t, "chk-1", results[0].Metadata.ChunkID)
	})
}

func TestQdrantVectorStore_DenseOnlyBackwardCompatible(t *testing.T) {
	ctx := context.Background()
	collections := &fakeCollectionsServer{}
	// NO WithSparseVectors: legacy dense-only store.
	qs := newTestQdrantStore(t, nil, collections)

	require.NoError(t, qs.UpsertPoints(ctx, []store.VectorPoint{{
		ID:     "pt-1",
		Vector: []float32{1.0, 0.0, 0.0},
		Metadata: model.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-1",
			ContentHash: "hash-1",
		},
	}}))

	t.Run("collection creation carries no sparse configuration", func(t *testing.T) {
		require.NotNil(t, collections.createReq)
		assert.Nil(t, collections.createReq.GetSparseVectorsConfig(),
			"dense-only collection must have no sparse vectors config")
	})

	t.Run("upsert stores dense vectors only", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{DocumentIDs: []string{"doc-1"}})
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.NotNil(t, listed[0].Vector, "dense vector must be present")
		assert.Nil(t, listed[0].Sparse, "no sparse vector must be introduced")
	})

	t.Run("dense search still works", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
		})
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "chk-1", results[0].Metadata.ChunkID)
	})
}

// qdrant.SearchPoints field check helper — compile-time documentation that the
// sparse search fields exist on the client we target.
