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
	contentStore := store.NewInMemoryContentStore()

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

	if err := contentStore.PutContent(ctx, []store.ChunkContent{
		{ChunkID: "chk-1", ContentMarkdown: "Clean architecture principles and deep modules."},
		{ChunkID: "chk-2", ContentMarkdown: "Database optimization and indexing strategies."},
	}); err != nil {
		t.Fatalf("failed to seed content store: %v", err)
	}

	denseRetriever := dense.NewDenseRetriever(mockProvider, storeImpl, contentStore)

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
		if results[0].ContentMarkdown == "" {
			t.Error("expected ContentMarkdown to be populated from ContentStore")
		}
		if results[0].ContentMarkdown != "Clean architecture principles and deep modules." {
			t.Errorf("unexpected ContentMarkdown: %q", results[0].ContentMarkdown)
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

func TestDenseRetrieverContentFromVectorPoints(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()
	// Deliberately EMPTY ContentStore: content must come from the vector points.
	contentStore := store.NewInMemoryContentStore()

	pt := store.VectorPoint{
		ID:              "pt-1",
		Vector:          mockProviderGenerateVector(mockProvider, "Rick Rubin founded Def Jam Recordings in New York."),
		ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons.",
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-1",
			SectionPath: "Everyone Is a Creator",
			ContentHash: "hash-1",
		},
	}
	if err := storeImpl.UpsertPoints(ctx, []store.VectorPoint{pt}); err != nil {
		t.Fatalf("failed to seed vector store: %v", err)
	}

	denseRetriever := dense.NewDenseRetriever(mockProvider, storeImpl, contentStore)

	results, err := denseRetriever.Retrieve(ctx, seam.RetrievalQuery{
		QueryText: "Rick Rubin founded Def Jam",
		TopK:      5,
		Mode:      seam.RetrievalDense,
	})
	if err != nil {
		t.Fatalf("unexpected error during retrieval: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
	if results[0].ContentMarkdown != "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons." {
		t.Errorf("expected content from vector point, got %q", results[0].ContentMarkdown)
	}
}

func mockProviderGenerateVector(p provider.EmbeddingProvider, text string) []float32 {
	vec, err := p.EmbedQuery(context.Background(), text)
	if err != nil || len(vec) == 0 {
		return make([]float32, 1536)
	}
	return vec
}
