package worker_test

import (
	"context"
	"testing"

	"arca/internal/indexing/job"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestIndexingWorker_ExecuteSync(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()
	w := worker.NewIndexingWorker(mockProvider, storeImpl)

	docID := "doc-e2e-123"
	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chk-1",
			ChunkOrder:      0,
			SectionPath:     "Introduction",
			ContentMarkdown: "Welcome to the system.",
			ContentHash:     "hash-1",
		},
		{
			ChunkID:         "chk-2",
			ChunkOrder:      1,
			SectionPath:     "Architecture",
			ContentMarkdown: "Architecture design overview.",
			ContentHash:     "hash-2",
		},
	}

	t.Run("successfully executes sync indexing job end to end", func(t *testing.T) {
		jobObj, err := w.ExecuteSync(ctx, docID, chunks)
		if err != nil {
			t.Fatalf("unexpected error during sync execution: %v", err)
		}

		if jobObj.Status != job.StatusCompleted {
			t.Errorf("expected job status Completed, got %s", jobObj.Status)
		}
		if jobObj.IndexedChunks != 2 {
			t.Errorf("expected 2 indexed chunks, got %d", jobObj.IndexedChunks)
		}

		// Verify points were stored in InMemoryVectorStore
		results, err := storeImpl.SearchVector(ctx, store.VectorSearchQuery{
			Vector: make([]float32, 1536),
			TopK:   10,
		})
		if err != nil {
			t.Fatalf("unexpected search error: %v", err)
		}

		if len(results) != 2 {
			t.Fatalf("expected 2 stored vector points, got %d", len(results))
		}
	})

	t.Run("skips unchanged chunks on re-index call", func(t *testing.T) {
		jobObj, err := w.ExecuteSync(ctx, docID, chunks)
		if err != nil {
			t.Fatalf("unexpected error during re-index execution: %v", err)
		}

		if jobObj.Status != job.StatusCompleted {
			t.Errorf("expected job status Completed, got %s", jobObj.Status)
		}
		if jobObj.SkippedChunks != 2 {
			t.Errorf("expected 2 skipped chunks, got %d", jobObj.SkippedChunks)
		}
		if jobObj.IndexedChunks != 0 {
			t.Errorf("expected 0 indexed chunks on re-index, got %d", jobObj.IndexedChunks)
		}
	})
}
