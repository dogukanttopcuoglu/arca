package cli

import (
	"testing"

	"arca/internal/indexing/provider"
)

func TestBuildEmbeddingProvider(t *testing.T) {
	t.Run("mock provider type returns mock embedding provider", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.EmbeddingProviderType = EmbeddingProviderMock
		cfg.EmbeddingModel = "mock-model-v1"

		p, err := buildEmbeddingProvider(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*provider.MockEmbeddingProvider); !ok {
			t.Fatalf("expected MockEmbeddingProvider, got %T", p)
		}
	})

	t.Run("nomic provider type returns ollama adapter", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.EmbeddingProviderType = EmbeddingProviderNomic
		cfg.EmbeddingModel = "nomic-embed-text:latest"
		cfg.OllamaBaseURL = "http://localhost:11434"

		p, err := buildEmbeddingProvider(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ollama, ok := p.(*provider.OllamaEmbeddingProvider)
		if !ok {
			t.Fatalf("expected OllamaEmbeddingProvider, got %T", p)
		}
		if ollama.Model() != "nomic-embed-text:latest" {
			t.Errorf("expected model nomic-embed-text:latest, got %q", ollama.Model())
		}
		if ollama.Capabilities().Dimension != 768 {
			t.Errorf("expected dimension 768, got %d", ollama.Capabilities().Dimension)
		}
	})
}
