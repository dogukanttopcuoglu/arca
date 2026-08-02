package dense_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/seam"
)

func TestDenseRetriever(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()

	// Seed vector points
	pt1 := store.VectorPoint{
		ID:     "pt-1",
		Vector: mockProviderGenerateVector(mockProvider, "Clean architecture principles and deep modules"),
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-1",
			SectionPath: "Architecture",
			ContentHash: "hash-1",
		},
	}
	pt2 := store.VectorPoint{
		ID:     "pt-2",
		Vector: mockProviderGenerateVector(mockProvider, "Database optimization and indexing strategies"),
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-2",
			SectionPath: "Database",
			ContentHash: "hash-2",
		},
	}

	if err := storeImpl.UpsertPoints(ctx, []store.VectorPoint{pt1, pt2}); err != nil {
		t.Fatalf("failed to seed vector store: %v", err)
	}

	denseRetriever := dense.NewDenseRetriever(mockProvider, storeImpl)

	t.Run("successfully retrieves nearest neighbor for query text", func(t *testing.T) {
		query := seam.RetrievalQuery{
			QueryText: "Clean architecture principles",
			TopK:      1,
			Mode:      seam.RetrievalDense,
		}

		results, err := denseRetriever.Retrieve(ctx, query)
		if err != nil {
			t.Fatalf("unexpected error during retrieval: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 search result, got %d", len(results))
		}
		if results[0].ChunkID != "chk-1" {
			t.Errorf("expected top match chk-1, got %s", results[0].ChunkID)
		}
		if results[0].Score <= 0 {
			t.Errorf("expected positive relevance score, got %.4f", results[0].Score)
		}
	})

	t.Run("applies metadata filter during retrieval", func(t *testing.T) {
		query := seam.RetrievalQuery{
			QueryText: "Clean architecture principles",
			TopK:      10,
			Mode:      seam.RetrievalDense,
			Filter:    indexingmodel.MetadataFilter{ChunkIDs: []string{"chk-2"}},
		}

		results, err := denseRetriever.Retrieve(ctx, query)
		if err != nil {
			t.Fatalf("unexpected error during retrieval: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 result matching chunk filter, got %d", len(results))
		}
		if results[0].ChunkID != "chk-2" {
			t.Errorf("expected chk-2, got %s", results[0].ChunkID)
		}
	})
}

func mockProviderGenerateVector(p provider.EmbeddingProvider, text string) []float32 {
	res, err := p.GenerateEmbeddings(context.Background(), []string{text})
	if err != nil || len(res.Vectors) == 0 {
		return make([]float32, 1536)
	}
	return res.Vectors[0]
}
