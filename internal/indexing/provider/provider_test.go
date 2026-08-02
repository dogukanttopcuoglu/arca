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

	t.Run("generates deterministic vectors for input text slice", func(t *testing.T) {
		ctx := context.Background()
		texts := []string{
			"First chunk content for embedding",
			"Second chunk content for embedding",
		}

		res, err := mock.GenerateEmbeddings(ctx, texts)
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
