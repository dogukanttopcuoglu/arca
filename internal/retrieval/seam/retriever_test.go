package seam_test

import (
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/retrieval/seam"
)

func TestRetrievalQueryValidation(t *testing.T) {
	t.Run("valid query passes validation", func(t *testing.T) {
		q := seam.RetrievalQuery{
			QueryText: "What is clean architecture?",
			TopK:      5,
			Mode:      seam.RetrievalDense,
		}
		if err := q.Validate(); err != nil {
			t.Errorf("expected valid query, got error: %v", err)
		}
	})

	t.Run("empty QueryText returns validation error", func(t *testing.T) {
		q := seam.RetrievalQuery{
			TopK: 5,
		}
		if err := q.Validate(); err == nil {
			t.Error("expected error for empty QueryText, got nil")
		}
	})

	t.Run("TopK < 1 defaults to 10 on normalization", func(t *testing.T) {
		q := seam.RetrievalQuery{
			QueryText: "Test query",
			TopK:      0,
		}
		q.Normalize()
		if q.TopK != 10 {
			t.Errorf("expected TopK normalized to 10, got %d", q.TopK)
		}
	})
}

func TestSearchResultSorting(t *testing.T) {
	t.Run("sorts SearchResults by score descending", func(t *testing.T) {
		results := []seam.SearchResult{
			{ChunkID: "chk-1", Score: 0.75},
			{ChunkID: "chk-2", Score: 0.95},
			{ChunkID: "chk-3", Score: 0.82},
		}

		seam.SortResultsByScore(results)

		if results[0].ChunkID != "chk-2" {
			t.Errorf("expected top result chk-2, got %s", results[0].ChunkID)
		}
		if results[1].ChunkID != "chk-3" {
			t.Errorf("expected second result chk-3, got %s", results[1].ChunkID)
		}
		if results[2].ChunkID != "chk-1" {
			t.Errorf("expected third result chk-1, got %s", results[2].ChunkID)
		}
	})
}

func TestSearchResultValidation(t *testing.T) {
	t.Run("valid SearchResult passes validation", func(t *testing.T) {
		res := seam.SearchResult{
			ChunkID: "chk-123",
			Score:   0.88,
			Metadata: indexingmodel.VectorMetadata{
				DocumentID: "doc-123",
				ChunkID:    "chk-123",
			},
		}
		if err := res.Validate(); err != nil {
			t.Errorf("expected valid search result, got error: %v", err)
		}
	})
}
