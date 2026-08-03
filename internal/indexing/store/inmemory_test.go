package store_test

import (
	"context"
	"testing"

	"arca/internal/indexing/model"
	"arca/internal/indexing/store"
)

func TestInMemoryVectorStore(t *testing.T) {
	ctx := context.Background()
	storeImpl := store.NewInMemoryVectorStore()

	t.Run("health check returns nil when active", func(t *testing.T) {
		if err := storeImpl.Health(ctx); err != nil {
			t.Errorf("expected healthy store, got error: %v", err)
		}
	})

	t.Run("upsert points inserts and updates points in place", func(t *testing.T) {
		points := []store.VectorPoint{
			{
				ID:     "pt-1",
				Vector: []float32{1.0, 0.0, 0.0},
				Metadata: model.VectorMetadata{
					DocumentID:  "doc-1",
					ChunkID:     "chk-1",
					ContentHash: "hash-v1",
				},
			},
			{
				ID:     "pt-2",
				Vector: []float32{0.0, 1.0, 0.0},
				Metadata: model.VectorMetadata{
					DocumentID:  "doc-1",
					ChunkID:     "chk-2",
					ContentHash: "hash-v1",
				},
			},
		}

		if err := storeImpl.UpsertPoints(ctx, points); err != nil {
			t.Fatalf("unexpected upsert error: %v", err)
		}

		// Inplace update of pt-1
		updatedPoint := store.VectorPoint{
			ID:     "pt-1",
			Vector: []float32{1.0, 0.0, 0.0},
			Metadata: model.VectorMetadata{
				DocumentID:  "doc-1",
				ChunkID:     "chk-1",
				ContentHash: "hash-v2",
			},
		}

		if err := storeImpl.UpsertPoints(ctx, []store.VectorPoint{updatedPoint}); err != nil {
			t.Fatalf("unexpected inplace upsert error: %v", err)
		}

		// Search doc-1
		results, err := storeImpl.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
			Filter: model.MetadataFilter{DocumentIDs: []string{"doc-1"}},
		})
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}

		if len(results) != 2 {
			t.Fatalf("expected 2 points for doc-1, got %d", len(results))
		}
		// First result should be pt-1 with cosine score ~1.0 and updated hash-v2
		if results[0].ID != "pt-1" {
			t.Errorf("expected top match pt-1, got %s", results[0].ID)
		}
		if results[0].Metadata.ContentHash != "hash-v2" {
			t.Errorf("expected inplace updated content hash 'hash-v2', got %q", results[0].Metadata.ContentHash)
		}
	})

	t.Run("metadata filter restricts search results", func(t *testing.T) {
		results, err := storeImpl.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
			Filter: model.MetadataFilter{ChunkIDs: []string{"chk-2"}},
		})
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 result for chunk filter chk-2, got %d", len(results))
		}
		if results[0].ID != "pt-2" {
			t.Errorf("expected pt-2, got %s", results[0].ID)
		}
	})

	t.Run("delete removes points matching metadata filter", func(t *testing.T) {
		err := storeImpl.Delete(ctx, model.MetadataFilter{
			DocumentIDs: []string{"doc-1"},
		})
		if err != nil {
			t.Fatalf("unexpected delete error: %v", err)
		}

		results, err := storeImpl.SearchVector(ctx, store.VectorSearchQuery{
			Vector: []float32{1.0, 0.0, 0.0},
			TopK:   10,
		})
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}

		if len(results) != 0 {
			t.Errorf("expected 0 points after deletion, got %d", len(results))
		}
	})

	t.Run("list points enumerates without ranking or truncation", func(t *testing.T) {
		points := []store.VectorPoint{
			{
				ID:     "pt-a",
				Vector: []float32{1.0, 0.0, 0.0},
				Metadata: model.VectorMetadata{
					DocumentID: "doc-list",
					ChunkID:    "chk-a",
				},
			},
			{
				ID:     "pt-b",
				Vector: []float32{0.0, 1.0, 0.0},
				Metadata: model.VectorMetadata{
					DocumentID: "doc-list",
					ChunkID:    "chk-b",
				},
			},
			{
				ID:     "pt-other",
				Vector: []float32{0.0, 0.0, 1.0},
				Metadata: model.VectorMetadata{
					DocumentID: "doc-other",
					ChunkID:    "chk-other",
				},
			},
		}
		if err := storeImpl.UpsertPoints(ctx, points); err != nil {
			t.Fatalf("unexpected upsert error: %v", err)
		}

		listed, err := storeImpl.ListPoints(ctx, model.MetadataFilter{DocumentIDs: []string{"doc-list"}})
		if err != nil {
			t.Fatalf("unexpected list error: %v", err)
		}
		if len(listed) != 2 {
			t.Fatalf("expected 2 listed points for doc-list, got %d", len(listed))
		}

		ids := map[string]bool{}
		for _, pt := range listed {
			ids[pt.ID] = true
			if len(pt.Vector) != 3 {
				t.Errorf("expected vector preserved in listed point %s", pt.ID)
			}
		}
		if !ids["pt-a"] || !ids["pt-b"] {
			t.Errorf("expected pt-a and pt-b in list, got %v", ids)
		}
	})
}
