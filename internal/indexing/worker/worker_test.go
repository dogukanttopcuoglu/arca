package worker_test

import (
	"context"
	"testing"

	"arca/internal/indexing/job"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestIndexingWorker_ExecuteSync(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	w := worker.NewIndexingWorker(mockProvider, storeImpl, contentStore)

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

func TestIndexingWorker_DeletesRemovedChunkPoints(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	w := worker.NewIndexingWorker(mockProvider, storeImpl, contentStore)

	docID := "doc-deletion-1"
	threeChunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chk-a",
			ChunkOrder:      0,
			SectionPath:     "Section A",
			ContentMarkdown: "Alpha content.",
			ContentHash:     "hash-a",
		},
		{
			ChunkID:         "chk-b",
			ChunkOrder:      1,
			SectionPath:     "Section B",
			ContentMarkdown: "Beta content.",
			ContentHash:     "hash-b",
		},
		{
			ChunkID:         "chk-c",
			ChunkOrder:      2,
			SectionPath:     "Section C",
			ContentMarkdown: "Gamma content.",
			ContentHash:     "hash-c",
		},
	}

	if _, err := w.ExecuteSync(ctx, docID, threeChunks); err != nil {
		t.Fatalf("initial index failed: %v", err)
	}

	// Section C is removed from the document; only A and B remain.
	remaining := []pdfmodel.KnowledgeChunk{threeChunks[0], threeChunks[1]}
	jobObj, err := w.ExecuteSync(ctx, docID, remaining)
	if err != nil {
		t.Fatalf("re-index failed: %v", err)
	}
	if jobObj.DeletedChunks != 1 {
		t.Errorf("expected 1 deleted chunk, got %d", jobObj.DeletedChunks)
	}

	results, err := storeImpl.SearchVector(ctx, store.VectorSearchQuery{
		Vector: make([]float32, 1536),
		TopK:   10,
		Filter: indexingmodel.MetadataFilter{DocumentIDs: []string{docID}},
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 remaining points after deletion, got %d", len(results))
	}

	removedPointID := store.CalculatePointID(docID, "Section C", 2)
	for _, r := range results {
		if r.ID == removedPointID {
			t.Errorf("deleted point %s should not remain in the store", removedPointID)
		}
	}
}

// recordingStore wraps InMemoryVectorStore and records which seam methods the worker invokes.
type recordingStore struct {
	inner          *store.InMemoryVectorStore
	listCalls      int
	searchCalls    int
}

func (s *recordingStore) UpsertPoints(ctx context.Context, points []store.VectorPoint) error {
	return s.inner.UpsertPoints(ctx, points)
}

func (s *recordingStore) SearchVector(ctx context.Context, query store.VectorSearchQuery) ([]store.VectorSearchResult, error) {
	s.searchCalls++
	return s.inner.SearchVector(ctx, query)
}

func (s *recordingStore) ListPoints(ctx context.Context, filter indexingmodel.MetadataFilter) ([]store.VectorPoint, error) {
	s.listCalls++
	return s.inner.ListPoints(ctx, filter)
}

func (s *recordingStore) Delete(ctx context.Context, filter indexingmodel.MetadataFilter) error {
	return s.inner.Delete(ctx, filter)
}

func (s *recordingStore) Health(ctx context.Context) error {
	return s.inner.Health(ctx)
}

func TestIndexingWorker_EnumeratesExistingPointsViaListPoints(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	spy := &recordingStore{inner: store.NewInMemoryVectorStore()}
	contentStore := store.NewInMemoryContentStore()
	w := worker.NewIndexingWorker(mockProvider, spy, contentStore)

	docID := "doc-enumeration-1"
	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chk-1",
			ChunkOrder:      0,
			SectionPath:     "Intro",
			ContentMarkdown: "Intro content.",
			ContentHash:     "hash-1",
		},
	}

	if _, err := w.ExecuteSync(ctx, docID, chunks); err != nil {
		t.Fatalf("unexpected error during sync execution: %v", err)
	}

	if spy.listCalls == 0 {
		t.Error("expected worker to enumerate existing points via ListPoints")
	}
	if spy.searchCalls != 0 {
		t.Errorf("expected no SearchVector calls during indexing, got %d", spy.searchCalls)
	}
}
