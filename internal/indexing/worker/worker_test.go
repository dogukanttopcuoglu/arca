package worker_test

import (
	"context"
	"strings"
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
		jobObj, err := w.ExecuteSync(ctx, docID, "Doc Title", chunks, nil)
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
		jobObj, err := w.ExecuteSync(ctx, docID, "Doc Title", chunks, nil)
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

	if _, err := w.ExecuteSync(ctx, docID, "Doc Title", threeChunks, nil); err != nil {
		t.Fatalf("initial index failed: %v", err)
	}

	// Section C is removed from the document; only A and B remain.
	remaining := []pdfmodel.KnowledgeChunk{threeChunks[0], threeChunks[1]}
	jobObj, err := w.ExecuteSync(ctx, docID, "Doc Title", remaining, nil)
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

func TestIndexingWorker_DocumentProfilePoint(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	w := worker.NewIndexingWorker(mockProvider, storeImpl, contentStore)

	docID := "doc-profile-1"
	meta := &pdfmodel.DocumentMetadata{
		Title:  "The Creative Act",
		Author: "Rick Rubin",
		Summary: &pdfmodel.Summary{
			Text:   "A book about creativity and art.",
			Source: pdfmodel.SummarySourceRuleBased,
		},
		Keywords: []pdfmodel.Keyword{
			{Value: "creativity", Score: 0.9},
			{Value: "art", Score: 0.8},
		},
	}
	chunks := []pdfmodel.KnowledgeChunk{
		{ChunkID: "chk-1", ChunkOrder: 1, SectionPath: "Document Overview", ContentMarkdown: "copyright page", ContentHash: "h1"},
		{ChunkID: "chk-2", ChunkOrder: 2, SectionPath: "Awareness", ContentMarkdown: "body one", ContentHash: "h2"},
		{ChunkID: "chk-3", ChunkOrder: 3, SectionPath: "Awareness", ContentMarkdown: "body two", ContentHash: "h3"},
	}

	profilePoints := func() []store.VectorPoint {
		t.Helper()
		points, err := storeImpl.ListPoints(ctx, indexingmodel.MetadataFilter{DocumentIDs: []string{docID}})
		if err != nil {
			t.Fatalf("ListPoints: %v", err)
		}
		var out []store.VectorPoint
		for _, p := range points {
			if p.Metadata.ContentType == pdfmodel.ContentTypeDocumentProfile {
				out = append(out, p)
			}
		}
		return out
	}

	t.Run("indexes one document profile point with the deterministic content", func(t *testing.T) {
		jobObj, err := w.ExecuteSync(ctx, docID, "The Creative Act", chunks, meta)
		if err != nil {
			t.Fatalf("ExecuteSync: %v", err)
		}
		if jobObj.Status != job.StatusCompleted {
			t.Fatalf("expected Completed, got %s", jobObj.Status)
		}

		profiles := profilePoints()
		if len(profiles) != 1 {
			t.Fatalf("expected exactly 1 profile point, got %d", len(profiles))
		}
		p := profiles[0]
		if p.Metadata.SectionPath != "Document Profile" || p.Metadata.ChunkOrder != 0 {
			t.Fatalf("profile metadata = %+v, want section 'Document Profile' / order 0", p.Metadata)
		}
		if p.Metadata.ChunkID != docID+"/document-profile/001" {
			t.Fatalf("profile chunk id = %q", p.Metadata.ChunkID)
		}
		if p.Metadata.ContentHash == "" || p.Metadata.IndexSignature == "" {
			t.Fatalf("profile must carry a stable ContentHash and IndexSignature, got %+v", p.Metadata)
		}
		if len(p.Vector) == 0 {
			t.Fatal("profile point must be embedded with the dense provider")
		}
		for _, want := range []string{
			"Title: The Creative Act",
			"Author: Rick Rubin",
			"Summary: A book about creativity and art.",
			"Key Topics: creativity, art",
			"Sections: Document Overview, Awareness",
		} {
			if !strings.Contains(p.ContentMarkdown, want) {
				t.Errorf("profile content missing %q:\n%s", want, p.ContentMarkdown)
			}
		}

		// Chunk points unchanged: 3 chunks + 1 profile.
		all, err := storeImpl.ListPoints(ctx, indexingmodel.MetadataFilter{DocumentIDs: []string{docID}})
		if err != nil {
			t.Fatalf("ListPoints: %v", err)
		}
		if len(all) != 4 {
			t.Fatalf("expected 4 points total (3 chunks + profile), got %d", len(all))
		}
	})

	t.Run("re-index is idempotent for the profile", func(t *testing.T) {
		first := profilePoints()[0]
		if _, err := w.ExecuteSync(ctx, docID, "The Creative Act", chunks, meta); err != nil {
			t.Fatalf("re-index: %v", err)
		}
		second := profilePoints()
		if len(second) != 1 {
			t.Fatalf("expected still exactly 1 profile point, got %d", len(second))
		}
		if second[0].Metadata.ContentHash != first.Metadata.ContentHash {
			t.Fatalf("unchanged metadata must keep the profile ContentHash (%s -> %s)", first.Metadata.ContentHash, second[0].Metadata.ContentHash)
		}
	})

	t.Run("changed profile content re-upserts the single point", func(t *testing.T) {
		before := profilePoints()[0].Metadata.ContentHash
		meta.Summary.Text = "An updated summary."
		if _, err := w.ExecuteSync(ctx, docID, "The Creative Act", chunks, meta); err != nil {
			t.Fatalf("re-index with changed summary: %v", err)
		}
		profiles := profilePoints()
		if len(profiles) != 1 {
			t.Fatalf("expected exactly 1 profile point after update, got %d", len(profiles))
		}
		if profiles[0].Metadata.ContentHash == before {
			t.Fatal("changed summary must change the profile ContentHash (re-upsert)")
		}
		if !strings.Contains(profiles[0].ContentMarkdown, "An updated summary.") {
			t.Fatalf("profile content not updated:\n%s", profiles[0].ContentMarkdown)
		}
	})

	t.Run("nil metadata produces no profile point", func(t *testing.T) {
		if _, err := w.ExecuteSync(ctx, "doc-no-profile", "", chunks, nil); err != nil {
			t.Fatalf("ExecuteSync: %v", err)
		}
		points, err := storeImpl.ListPoints(ctx, indexingmodel.MetadataFilter{DocumentIDs: []string{"doc-no-profile"}})
		if err != nil {
			t.Fatalf("ListPoints: %v", err)
		}
		for _, p := range points {
			if p.Metadata.ContentType == pdfmodel.ContentTypeDocumentProfile {
				t.Fatalf("nil metadata must not create a profile point, got %+v", p.Metadata)
			}
		}
	})

	t.Run("document deletion removes the profile with the document", func(t *testing.T) {
		if err := storeImpl.Delete(ctx, indexingmodel.MetadataFilter{DocumentIDs: []string{docID}}); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if profiles := profilePoints(); len(profiles) != 0 {
			t.Fatalf("profile must be deleted with the document, got %d", len(profiles))
		}
	})
}

func TestIndexingWorker_ProfileWithSparseProvider(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	sparseProvider := &fakeSparseProvider{}
	w := worker.NewIndexingWorker(mockProvider, storeImpl, contentStore, worker.WithSparseEncoderProvider(sparseProvider))

	meta := &pdfmodel.DocumentMetadata{
		Title:  "The Creative Act",
		Author: "Rick Rubin",
		Summary: &pdfmodel.Summary{
			Text:   "A book about creativity and art.",
			Source: pdfmodel.SummarySourceRuleBased,
		},
	}
	chunks := []pdfmodel.KnowledgeChunk{
		{ChunkID: "chk-1", ChunkOrder: 1, SectionPath: "Intro", ContentMarkdown: "body", ContentHash: "h1"},
	}

	jobObj, err := w.ExecuteSync(ctx, "doc-sparse-1", "The Creative Act", chunks, meta)
	if err != nil {
		t.Fatalf("ExecuteSync: %v", err)
	}
	if jobObj.Status != job.StatusCompleted {
		t.Fatalf("expected Completed, got %s", jobObj.Status)
	}

	points, err := storeImpl.ListPoints(ctx, indexingmodel.MetadataFilter{DocumentIDs: []string{"doc-sparse-1"}})
	if err != nil {
		t.Fatalf("ListPoints: %v", err)
	}
	var profile, chunk *store.VectorPoint
	for i := range points {
		if points[i].Metadata.ContentType == pdfmodel.ContentTypeDocumentProfile {
			profile = &points[i]
		} else {
			chunk = &points[i]
		}
	}
	if profile == nil || chunk == nil {
		t.Fatalf("expected one profile and one chunk point, got %d points", len(points))
	}
	// The chunk carries the sparse vector; the profile must NOT (ADR-0048:
	// the profile never shifts BM25 IDF statistics).
	if chunk.Sparse == nil {
		t.Fatal("chunk point must carry the sparse vector")
	}
	if profile.Sparse != nil {
		t.Fatal("profile point must never carry a sparse vector")
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

	if _, err := w.ExecuteSync(ctx, docID, "Doc Title", chunks, nil); err != nil {
		t.Fatalf("unexpected error during sync execution: %v", err)
	}

	if spy.listCalls == 0 {
		t.Error("expected worker to enumerate existing points via ListPoints")
	}
	if spy.searchCalls != 0 {
		t.Errorf("expected no SearchVector calls during indexing, got %d", spy.searchCalls)
	}
}
