package hybrid_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/hybrid"
	"arca/internal/retrieval/seam"
)

func TestReciprocalRankFusion(t *testing.T) {
	t.Run("merges and ranks items from disjoint retriever streams using RRF", func(t *testing.T) {
		stream1 := []seam.SearchResult{
			{ChunkID: "chk-A", Score: 0.90},
			{ChunkID: "chk-B", Score: 0.80},
		}
		stream2 := []seam.SearchResult{
			{ChunkID: "chk-B", Score: 0.95},
			{ChunkID: "chk-C", Score: 0.85},
		}

		fused := hybrid.ReciprocalRankFusion([][]seam.SearchResult{stream1, stream2}, 60)

		if len(fused) != 3 {
			t.Fatalf("expected 3 fused results, got %d", len(fused))
		}

		// chk-B appeared in both streams (rank 2 in stream1, rank 1 in stream2) so it should rank #1 overall
		if fused[0].ChunkID != "chk-B" {
			t.Errorf("expected top fused result chk-B, got %s", fused[0].ChunkID)
		}
	})
}

func TestHybridRetriever(t *testing.T) {
	ctx := context.Background()

	mockProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	storeImpl := store.NewInMemoryVectorStore()

	// Seed point
	pt := store.VectorPoint{
		ID:     "pt-1",
		Vector: mockProviderGenerateVector(mockProvider, "Hybrid retrieval and RRF fusion"),
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  "doc-1",
			ChunkID:     "chk-1",
			SectionPath: "Retrieval",
			ContentHash: "hash-1",
		},
	}
	_ = storeImpl.UpsertPoints(ctx, []store.VectorPoint{pt})

	denseRetriever := dense.NewDenseRetriever(mockProvider, storeImpl, store.NewInMemoryContentStore())
	sparseRetriever := &MockSparseRetriever{
		results: []seam.SearchResult{
			{
				ChunkID: "chk-1",
				Score:   0.88,
				Metadata: indexingmodel.VectorMetadata{
					DocumentID: "doc-1",
					ChunkID:    "chk-1",
				},
			},
		},
	}

	hybridRetriever := hybrid.NewHybridRetriever(denseRetriever, sparseRetriever)

	t.Run("successfully executes hybrid retrieval merging dense and sparse results", func(t *testing.T) {
		query := seam.RetrievalQuery{
			QueryText: "Hybrid retrieval and RRF fusion",
			TopK:      5,
			Mode:      seam.RetrievalHybrid,
		}

		results, err := hybridRetriever.Retrieve(ctx, query)
		if err != nil {
			t.Fatalf("unexpected error during hybrid retrieval: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("expected non-empty hybrid search results")
		}
		if results[0].ChunkID != "chk-1" {
			t.Errorf("expected top hybrid result chk-1, got %s", results[0].ChunkID)
		}
	})
}

type MockSparseRetriever struct {
	results []seam.SearchResult
}

func (m *MockSparseRetriever) Retrieve(ctx context.Context, query seam.RetrievalQuery) ([]seam.SearchResult, error) {
	return m.results, nil
}

func mockProviderGenerateVector(p provider.EmbeddingProvider, text string) []float32 {
	vec, err := p.EmbedQuery(context.Background(), text)
	if err != nil || len(vec) == 0 {
		return make([]float32, 1536)
	}
	return vec
}
