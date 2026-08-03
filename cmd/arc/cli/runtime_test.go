package cli

import (
	"testing"

	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
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

func TestBuildVectorStore(t *testing.T) {
	t.Run("inmemory store type returns in-memory store", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.VectorStoreType = VectorStoreInMemory

		s, err := buildVectorStore(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := s.(*store.InMemoryVectorStore); !ok {
			t.Fatalf("expected InMemoryVectorStore, got %T", s)
		}
	})

	t.Run("qdrant store type returns qdrant adapter", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.VectorStoreType = VectorStoreQdrant
		cfg.VectorStoreURL = "http://localhost:6334"
		cfg.QdrantCollection = "arca_chunks"

		s, err := buildVectorStore(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := s.(*store.QdrantVectorStore); !ok {
			t.Fatalf("expected QdrantVectorStore, got %T", s)
		}
	})
}
