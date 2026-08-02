package store_test

import (
	"testing"

	"arca/internal/indexing/model"
	"arca/internal/indexing/store"
)

func TestCalculatePointID(t *testing.T) {
	t.Run("generates deterministic point ID for document section and chunk order", func(t *testing.T) {
		id1 := store.CalculatePointID("doc-123", "Architecture Overview/Subsystems", 1)
		id2 := store.CalculatePointID("doc-123", "Architecture Overview/Subsystems", 1)

		if id1 == "" {
			t.Fatal("expected non-empty point ID")
		}
		if id1 != id2 {
			t.Errorf("expected deterministic point IDs, got %q vs %q", id1, id2)
		}
	})

	t.Run("point ID remains stable across content revisions when order and path match", func(t *testing.T) {
		idVersion1 := store.CalculatePointID("doc-123", "Introduction", 0)
		idVersion2 := store.CalculatePointID("doc-123", "Introduction", 0)

		if idVersion1 != idVersion2 {
			t.Errorf("expected Point ID stability across content revisions, got %q vs %q", idVersion1, idVersion2)
		}
	})

	t.Run("point ID changes when document section path or order changes", func(t *testing.T) {
		id1 := store.CalculatePointID("doc-123", "Introduction", 0)
		id2 := store.CalculatePointID("doc-123", "Introduction", 1)
		id3 := store.CalculatePointID("doc-999", "Introduction", 0)

		if id1 == id2 {
			t.Error("expected different point ID when chunk order changes")
		}
		if id1 == id3 {
			t.Error("expected different point ID when document ID changes")
		}
	})
}

func TestVectorPointValidation(t *testing.T) {
	t.Run("valid vector point passes validation", func(t *testing.T) {
		pt := store.VectorPoint{
			ID:     "pt-123",
			Vector: []float32{0.1, 0.2, 0.3},
			Metadata: model.VectorMetadata{
				DocumentID: "doc-123",
				ChunkID:    "chk-1",
			},
		}
		if err := pt.Validate(); err != nil {
			t.Errorf("expected valid vector point, got error: %v", err)
		}
	})

	t.Run("empty vector point ID returns validation error", func(t *testing.T) {
		pt := store.VectorPoint{
			Vector: []float32{0.1},
		}
		if err := pt.Validate(); err == nil {
			t.Error("expected error for empty point ID, got nil")
		}
	})

	t.Run("empty vector slice returns validation error", func(t *testing.T) {
		pt := store.VectorPoint{
			ID: "pt-123",
		}
		if err := pt.Validate(); err == nil {
			t.Error("expected error for empty vector slice, got nil")
		}
	})
}
