package provider_test

import (
	"context"
	"testing"

	"arca/internal/indexing/provider"
)

func TestMockEmbeddingProvider(t *testing.T) {
	mock := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)

	t.Run("returns correct provider capabilities", func(t *testing.T) {
		caps := mock.Capabilities()
		if caps.Dimension != 1536 {
			t.Errorf("expected dimension 1536, got %d", caps.Dimension)
		}
		if caps.MaxBatchSize <= 0 {
			t.Errorf("expected MaxBatchSize > 0, got %d", caps.MaxBatchSize)
		}
	})

	t.Run("embeds document texts in batch", func(t *testing.T) {
		ctx := context.Background()
		texts := []string{
			"First chunk content for embedding",
			"Second chunk content for embedding",
		}

		res, err := mock.EmbedDocuments(ctx, texts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res == nil {
			t.Fatal("expected non-nil EmbeddingResult")
		}
		if len(res.Vectors) != 2 {
			t.Fatalf("expected 2 vectors, got %d", len(res.Vectors))
		}
		if len(res.Vectors[0]) != 1536 {
			t.Errorf("expected vector dimension 1536, got %d", len(res.Vectors[0]))
		}
		if res.Provider != "mock-provider" {
			t.Errorf("expected provider 'mock-provider', got %q", res.Provider)
		}
		if res.Model != "mock-model-v1" {
			t.Errorf("expected model 'mock-model-v1', got %q", res.Model)
		}
		if res.Usage.TotalTokens <= 0 {
			t.Errorf("expected TotalTokens > 0, got %d", res.Usage.TotalTokens)
		}
	})

	t.Run("embeds a single query vector", func(t *testing.T) {
		ctx := context.Background()

		vec, err := mock.EmbedQuery(ctx, "vector search query")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vec) != 1536 {
			t.Errorf("expected query vector dimension 1536, got %d", len(vec))
		}
	})

	t.Run("returns deterministic document embeddings", func(t *testing.T) {
		ctx := context.Background()

		a, err := mock.EmbedDocuments(ctx, []string{"deterministic text"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		b, err := mock.EmbedDocuments(ctx, []string{"deterministic text"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(a.Vectors[0]) != len(b.Vectors[0]) {
			t.Fatal("expected matching vector lengths")
		}
		for i := range a.Vectors[0] {
			if a.Vectors[0][i] != b.Vectors[0][i] {
				t.Fatalf("expected deterministic vectors, mismatch at index %d", i)
			}
		}
	})

	t.Run("health check returns nil when healthy", func(t *testing.T) {
		ctx := context.Background()
		if err := mock.Health(ctx); err != nil {
			t.Errorf("expected healthy state, got error: %v", err)
		}
	})

	t.Run("health check returns error when simulated unhealthy", func(t *testing.T) {
		ctx := context.Background()
		mock.SetHealthy(false)
		if err := mock.Health(ctx); err == nil {
			t.Error("expected error for unhealthy state, got nil")
		}
	})
}
