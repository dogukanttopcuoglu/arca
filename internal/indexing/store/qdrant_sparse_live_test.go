package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
)

// TestQdrantVectorStore_SparseLive runs against a real Qdrant instance when
// QDRANT_TEST_URL is set. It creates a sparse-enabled collection, upserts
// points with dense + sparse vectors, searches by sparse vector, and cleans up.
func TestQdrantVectorStore_SparseLive(t *testing.T) {
	host := os.Getenv("QDRANT_TEST_URL")
	if host == "" {
		t.Skip("QDRANT_TEST_URL not set; skipping live sparse Qdrant test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collection := "arca_test_sparse_integration"
	qs, err := store.NewQdrantVectorStore(host, collection, store.WithQdrantDimension(3), store.WithSparseVectors())
	if err != nil {
		t.Fatalf("failed to construct Qdrant store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })

	points := []store.VectorPoint{
		{
			ID:     "00000000-0000-0000-0000-000000000011",
			Vector: []float32{1.0, 0.0, 0.0},
			Sparse: &sparse.SparseVector{Indices: []uint32{1, 3}, Values: []float32{2.0, 4.0}},
			Metadata: model.VectorMetadata{
				DocumentID:  "sparse-doc-1",
				ChunkID:     "sparse-chk-1",
				ContentHash: "sparse-hash-1",
			},
		},
		{
			ID:     "00000000-0000-0000-0000-000000000012",
			Vector: []float32{0.0, 1.0, 0.0},
			Sparse: &sparse.SparseVector{Indices: []uint32{1, 5}, Values: []float32{8.0, 1.0}},
			Metadata: model.VectorMetadata{
				DocumentID:  "sparse-doc-1",
				ChunkID:     "sparse-chk-2",
				ContentHash: "sparse-hash-2",
			},
		},
	}
	if err := qs.UpsertPoints(ctx, points); err != nil {
		t.Fatalf("sparse upsert failed: %v", err)
	}

	t.Run("sparse search returns points by sparse similarity", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Sparse: &sparse.SparseVector{Indices: []uint32{1}, Values: []float32{1.0}},
			TopK:   10,
			Filter: model.MetadataFilter{DocumentIDs: []string{"sparse-doc-1"}},
		})
		if err != nil {
			t.Fatalf("sparse search failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected sparse search results")
		}
		// The point with value 8.0 at index 1 dominates the dot product.
		if results[0].Metadata.ChunkID != "sparse-chk-2" {
			t.Errorf("expected sparse-chk-2 as top hit, got %s", results[0].Metadata.ChunkID)
		}
	})

	t.Run("list preserves both dense and sparse vectors", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{DocumentIDs: []string{"sparse-doc-1"}})
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(listed) != 2 {
			t.Fatalf("expected 2 listed points, got %d", len(listed))
		}
		for _, pt := range listed {
			if len(pt.Vector) != 3 {
				t.Errorf("expected 3-dim dense vector, got %d", len(pt.Vector))
			}
			if pt.Sparse == nil || len(pt.Sparse.Indices) == 0 {
				t.Errorf("expected sparse vector on %s", pt.Metadata.ChunkID)
			}
		}
	})

	t.Run("dense search still works on the sparse-enabled collection", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
		})
		if err != nil {
			t.Fatalf("dense search failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 dense results, got %d", len(results))
		}
	})
}
