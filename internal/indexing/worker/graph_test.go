package worker_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	graphmodel "arca/internal/graph/model"
	graphstore "arca/internal/graph/store"
	"arca/internal/indexing/job"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
	pdfmodel "arca/internal/pdfinspector/model"
)

func entityChunk(id, section string, order int, content string, entities ...pdfmodel.EntityMention) pdfmodel.KnowledgeChunk {
	return pdfmodel.KnowledgeChunk{
		ChunkID:         id,
		ChunkOrder:      order,
		SectionPath:     section,
		ContentMarkdown: content,
		ContentHash:     "hash-" + id,
		Entities:        entities,
	}
}

func mention(text string, entityType pdfmodel.EntityType) pdfmodel.EntityMention {
	return pdfmodel.EntityMention{Text: text, Type: entityType}
}

func TestIndexingWorker_GraphWrites(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	vecStore := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	gs := graphstore.NewInMemoryGraphStore()
	w := worker.NewIndexingWorker(mockProvider, vecStore, contentStore, worker.WithGraphStore(gs))

	t.Run("writes entities at the 0.90 floor with cross-chunk scoring", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			entityChunk("doc-x/notes/001", "Notes", 0, "World Bank lending.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
			entityChunk("doc-x/notes/002", "Notes", 1, "World Bank report.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
			entityChunk("doc-x/notes/003", "Notes", 2, "A passing mention.",
				mention("Oxford University", pdfmodel.EntityTypeOrganization)),
		}
		if _, err := w.ExecuteSync(ctx, "doc-x", chunks); err != nil {
			t.Fatalf("execute: %v", err)
		}

		// World Bank: 2 mentions -> score 0.9 -> persisted with both chunks.
		node, err := gs.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("expected world bank node: %v", err)
		}
		if node.ID != "organization:world bank" {
			t.Errorf("expected deterministic node id, got %q", node.ID)
		}
		if len(node.ChunkIDs()) != 2 {
			t.Errorf("expected 2 evidenced chunks, got %v", node.ChunkIDs())
		}
		if node.Score() < 0.9 {
			t.Errorf("expected score >= 0.9, got %v", node.Score())
		}

		// Oxford: 1 mention -> score 0.8 -> below floor -> absent.
		if _, err := gs.FindNodeByName(ctx, "Oxford University"); err == nil {
			t.Error("expected single-mention entity below the 0.90 floor to be absent")
		}
	})

	t.Run("re-index of unchanged chunks stays idempotent", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			entityChunk("doc-x/notes/001", "Notes", 0, "World Bank lending.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
			entityChunk("doc-x/notes/002", "Notes", 1, "World Bank report.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
		}
		before, err := gs.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("node before re-index: %v", err)
		}
		if _, err := w.ExecuteSync(ctx, "doc-x", chunks); err != nil {
			t.Fatalf("re-index: %v", err)
		}
		node, err := gs.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("node after re-index: %v", err)
		}
		if len(node.ChunkIDs()) != 2 {
			t.Errorf("expected unchanged evidence after re-index, got %v", node.ChunkIDs())
		}
		if node.Score() != before.Score() {
			t.Errorf("expected unchanged score after skip, got %v vs %v", node.Score(), before.Score())
		}
	})

	t.Run("partial-set writes never regress the score", func(t *testing.T) {
		// A fresh document, so prior subtests cannot pollute the graph state.
		chunks := []pdfmodel.KnowledgeChunk{
			entityChunk("doc-w/notes/001", "Notes", 0, "World Bank lending.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
			entityChunk("doc-w/notes/002", "Notes", 1, "World Bank report.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
			entityChunk("doc-w/notes/003", "Notes", 2, "World Bank policy.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
		}
		if _, err := w.ExecuteSync(ctx, "doc-w", chunks); err != nil {
			t.Fatalf("full index: %v", err)
		}
		node, err := gs.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("node after full index: %v", err)
		}
		if node.Score() != 1.0 {
			t.Fatalf("expected score 1.0 with three mentions, got %v", node.Score())
		}

		// Same document, only notes/003 modified (different content).
		modified := []pdfmodel.KnowledgeChunk{
			entityChunk("doc-w/notes/001", "Notes", 0, "World Bank lending.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
			entityChunk("doc-w/notes/002", "Notes", 1, "World Bank report.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
			entityChunk("doc-w/notes/003", "Notes", 2, "World Bank policy updated.",
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
		}
		if _, err := w.ExecuteSync(ctx, "doc-w", modified); err != nil {
			t.Fatalf("partial re-index: %v", err)
		}
		node, err = gs.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("node after partial re-index: %v", err)
		}
		if node.Score() != 1.0 {
			t.Errorf("expected score preserved at 1.0 on partial write, got %v", node.Score())
		}
		// Cross-document union: doc-x evidence from earlier subtests remains;
		// every doc-w chunk must be evidenced.
		if len(node.ChunkIDs()) < 3 {
			t.Errorf("expected at least the three doc-w chunks evidenced, got %v", node.ChunkIDs())
		}
		for _, cid := range []string{"doc-w/notes/001", "doc-w/notes/002", "doc-w/notes/003"} {
			if !slices.Contains(node.ChunkIDs(), cid) {
				t.Errorf("expected %s evidenced, got %v", cid, node.ChunkIDs())
			}
		}
	})

	t.Run("removed chunks drop their graph evidence", func(t *testing.T) {
		// Re-index with notes/002 removed. The remaining chunk carries two
		// mentions, so the entity stays at score 0.9 and survives with only
		// its remaining evidence.
		chunks := []pdfmodel.KnowledgeChunk{
			entityChunk("doc-x/notes/001", "Notes", 0, "World Bank lending and World Bank policy.",
				mention("World Bank", pdfmodel.EntityTypeOrganization),
				mention("World Bank", pdfmodel.EntityTypeOrganization)),
		}
		if _, err := w.ExecuteSync(ctx, "doc-x", chunks); err != nil {
			t.Fatalf("re-index with removal: %v", err)
		}
		node, err := gs.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("node after removal: %v", err)
		}
		// The removed doc-x chunk must drop its evidence; other documents'
		// evidence (doc-w) survives the document-scoped cleanup.
		for _, cid := range node.ChunkIDs() {
			if cid == "doc-x/notes/002" {
				t.Errorf("expected removed chunk evidence dropped, got %v", node.ChunkIDs())
			}
		}
		if !slices.Contains(node.ChunkIDs(), "doc-x/notes/001") {
			t.Errorf("expected remaining doc-x evidence, got %v", node.ChunkIDs())
		}
	})
}

func TestIndexingWorker_GraphWritesOffByDefault(t *testing.T) {
	ctx := context.Background()
	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	w := worker.NewIndexingWorker(mockProvider, store.NewInMemoryVectorStore(), store.NewInMemoryContentStore())

	chunks := []pdfmodel.KnowledgeChunk{
		entityChunk("doc-y/body/001", "Body", 0, "Content with an entity.",
			mention("World Bank", pdfmodel.EntityTypeOrganization)),
	}
	if _, err := w.ExecuteSync(ctx, "doc-y", chunks); err != nil {
		t.Fatalf("execute without graph store: %v", err)
	}
}

// failingGraphStore fails every write, simulating a broken graph backend.
type failingGraphStore struct{}

func (failingGraphStore) AddNode(context.Context, graphmodel.Node) error {
	return errors.New("graph backend down")
}
func (failingGraphStore) AddEdge(context.Context, graphmodel.Edge) error {
	return errors.New("edges not supported")
}
func (failingGraphStore) GetNode(context.Context, string) (*graphmodel.Node, error) {
	return nil, errors.New("node not found")
}
func (failingGraphStore) FindNodeByName(context.Context, string) (*graphmodel.Node, error) {
	return nil, errors.New("node not found")
}
func (failingGraphStore) ListEntityNodes(context.Context) ([]graphmodel.Node, error) {
	return nil, errors.New("graph backend down")
}
func (failingGraphStore) DeleteByDocument(context.Context, string) error {
	return errors.New("graph backend down")
}
func (failingGraphStore) Traverse(context.Context, string, int) ([]graphmodel.Node, error) {
	return nil, errors.New("not supported")
}

func TestIndexingWorker_GraphFailureFailsJobBeforeVectorUpsert(t *testing.T) {
	ctx := context.Background()
	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	vecStore := store.NewInMemoryVectorStore()
	w := worker.NewIndexingWorker(mockProvider, vecStore, store.NewInMemoryContentStore(),
		worker.WithGraphStore(failingGraphStore{}))

	chunks := []pdfmodel.KnowledgeChunk{
		// Two mentions: above the 0.90 floor, so AddNode is actually reached.
		entityChunk("doc-z/body/001", "Body", 0, "World Bank lending and World Bank policy.",
			mention("World Bank", pdfmodel.EntityTypeOrganization),
			mention("World Bank", pdfmodel.EntityTypeOrganization)),
	}
	jobObj, err := w.ExecuteSync(ctx, "doc-z", chunks)
	if err == nil {
		t.Fatal("expected graph failure to fail the job")
	}
	if jobObj.Status != job.StatusFailed {
		t.Errorf("expected job status Failed, got %s", jobObj.Status)
	}
	// The graph write happens before the vector upsert, so a graph failure
	// must leave the vector collection untouched: a retry re-runs the same
	// diff instead of permanently skipping the graph write.
	results, serr := vecStore.SearchVector(ctx, store.VectorSearchQuery{Vector: make([]float32, 1536), TopK: 10})
	if serr != nil {
		t.Fatalf("search: %v", serr)
	}
	if len(results) != 0 {
		t.Errorf("expected no vector points after graph failure, got %d", len(results))
	}
}
