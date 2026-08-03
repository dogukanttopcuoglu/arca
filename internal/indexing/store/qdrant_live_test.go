package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"arca/internal/indexing/model"
	"arca/internal/indexing/store"
)

// TestQdrantVectorStore_Live runs against a real Qdrant instance when
// QDRANT_TEST_URL is set (e.g. "localhost:6334"). It is skipped otherwise.
func TestQdrantVectorStore_Live(t *testing.T) {
	host := os.Getenv("QDRANT_TEST_URL")
	if host == "" {
		t.Skip("QDRANT_TEST_URL not set; skipping live Qdrant integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collection := "arca_test_integration"
	qs, err := store.NewQdrantVectorStore(host, collection)
	if err != nil {
		t.Fatalf("failed to construct Qdrant store: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort cleanup; ignore errors if the collection persists for inspection.
		_ = qs.Close()
	})

	if err := qs.Health(ctx); err != nil {
		t.Fatalf("Qdrant health check failed (is QDRANT_TEST_URL reachable?): %v", err)
	}

	points := []store.VectorPoint{
		{
			ID:     "live-pt-1",
			Vector: []float32{1.0, 0.0, 0.0},
			Metadata: model.VectorMetadata{
				DocumentID:  "live-doc-1",
				ChunkID:     "live-chk-1",
				ChunkOrder:  0,
				SectionPath: "Live Section",
				PageNumbers: []int{1},
				ContentType: "paragraph",
				ContentHash: "live-hash-1",
			},
		},
		{
			ID:     "live-pt-2",
			Vector: []float32{0.0, 1.0, 0.0},
			Metadata: model.VectorMetadata{
				DocumentID:  "live-doc-1",
				ChunkID:     "live-chk-2",
				ChunkOrder:  1,
				SectionPath: "Other Section",
				PageNumbers: []int{2},
				ContentType: "paragraph",
				ContentHash: "live-hash-2",
			},
		},
	}

	if err := qs.UpsertPoints(ctx, points); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	t.Run("search returns indexed points", func(t *testing.T) {
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
		})
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 search results, got %d", len(results))
		}
	})

	t.Run("list points enumerates all with vectors", func(t *testing.T) {
		listed, err := qs.ListPoints(ctx, model.MetadataFilter{DocumentIDs: []string{"live-doc-1"}})
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(listed) != 2 {
			t.Fatalf("expected 2 listed points, got %d", len(listed))
		}
		for _, pt := range listed {
			if len(pt.Vector) != 3 {
				t.Fatalf("expected 3-dim vector, got %d", len(pt.Vector))
			}
		}
	})

	t.Run("delete removes points by filter", func(t *testing.T) {
		if err := qs.Delete(ctx, model.MetadataFilter{DocumentIDs: []string{"live-doc-1"}}); err != nil {
			t.Fatalf("delete failed: %v", err)
		}
		results, err := qs.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
		})
		if err != nil {
			t.Fatalf("search after delete failed: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results after delete, got %d", len(results))
		}
	})
}
