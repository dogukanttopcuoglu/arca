package diff_test

import (
	"testing"

	"arca/internal/indexing/diff"
	indexingmodel "arca/internal/indexing/model"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestDiffEngine(t *testing.T) {
	engine := diff.NewEngine("OpenAI", "text-embedding-3-large", "v1", "1.0")

	docID := "doc-123"
	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chk-1",
			ChunkOrder:      0,
			SectionPath:     "Introduction",
			ContentMarkdown: "Same intro content",
			ContentHash:     "hash-intro",
		},
		{
			ChunkID:         "chk-2",
			ChunkOrder:      1,
			SectionPath:     "Overview",
			ContentMarkdown: "Updated overview content",
			ContentHash:     "hash-overview-v2",
		},
		{
			ChunkID:         "chk-3",
			ChunkOrder:      2,
			SectionPath:     "New Section",
			ContentMarkdown: "Newly added section content",
			ContentHash:     "hash-new",
		},
	}

	// Signature calculation for unchanged chk-1
	introSig := indexingmodel.CalculateIndexSignature("hash-intro", "OpenAI", "text-embedding-3-large", "v1", "1.0")
	// Signature calculation for old chk-2
	oldOverviewSig := indexingmodel.CalculateIndexSignature("hash-overview-v1", "OpenAI", "text-embedding-3-large", "v1", "1.0")
	// Signature calculation for deleted section
	deletedSig := indexingmodel.CalculateIndexSignature("hash-deleted", "OpenAI", "text-embedding-3-large", "v1", "1.0")

	existingPoints := []indexingmodel.VectorMetadata{
		{
			DocumentID:        docID,
			ChunkID:           "chk-1",
			ChunkOrder:        0,
			SectionPath:       "Introduction",
			ContentHash:       "hash-intro",
			IndexSignature:    introSig,
			EmbeddingProvider: "OpenAI",
			EmbeddingModel:    "text-embedding-3-large",
		},
		{
			DocumentID:        docID,
			ChunkID:           "chk-2",
			ChunkOrder:        1,
			SectionPath:       "Overview",
			ContentHash:       "hash-overview-v1",
			IndexSignature:    oldOverviewSig,
			EmbeddingProvider: "OpenAI",
			EmbeddingModel:    "text-embedding-3-large",
		},
		{
			DocumentID:        docID,
			ChunkID:           "chk-deleted-old",
			ChunkOrder:        2,
			SectionPath:       "Removed Section",
			ContentHash:       "hash-deleted",
			IndexSignature:    deletedSig,
			EmbeddingProvider: "OpenAI",
			EmbeddingModel:    "text-embedding-3-large",
		},
	}

	plan := engine.ComputeDiffPlan(docID, chunks, existingPoints)

	if plan == nil {
		t.Fatal("expected non-nil DiffPlan")
	}

	t.Run("skips unchanged chunk chk-1", func(t *testing.T) {
		if len(plan.UnchangedChunks) != 1 {
			t.Errorf("expected 1 unchanged chunk, got %d", len(plan.UnchangedChunks))
		}
		if len(plan.UnchangedChunks) > 0 && plan.UnchangedChunks[0].ChunkID != "chk-1" {
			t.Errorf("expected chk-1 to be unchanged, got %s", plan.UnchangedChunks[0].ChunkID)
		}
	})

	t.Run("flags modified chunk chk-2 for re-embedding", func(t *testing.T) {
		if len(plan.ModifiedChunks) != 1 {
			t.Errorf("expected 1 modified chunk, got %d", len(plan.ModifiedChunks))
		}
		if len(plan.ModifiedChunks) > 0 && plan.ModifiedChunks[0].ChunkID != "chk-2" {
			t.Errorf("expected chk-2 to be modified, got %s", plan.ModifiedChunks[0].ChunkID)
		}
	})

	t.Run("flags newly added chunk chk-3 for embedding", func(t *testing.T) {
		if len(plan.NewChunks) != 1 {
			t.Errorf("expected 1 new chunk, got %d", len(plan.NewChunks))
		}
		if len(plan.NewChunks) > 0 && plan.NewChunks[0].ChunkID != "chk-3" {
			t.Errorf("expected chk-3 to be new, got %s", plan.NewChunks[0].ChunkID)
		}
	})

	t.Run("flags removed old point for deletion", func(t *testing.T) {
		if len(plan.DeletedPointIDs) != 1 {
			t.Errorf("expected 1 deleted point ID, got %d", len(plan.DeletedPointIDs))
		}
	})
}
