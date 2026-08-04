package cli

import (
	"context"
	"strings"
	"testing"

	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	llmprovider "arca/internal/llm/provider"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/seam"
)

func TestDefaultConfig_LLMSettings(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.LLMBaseURL != "https://agentrouter.org/v1" {
		t.Errorf("expected ADR-0026 default base URL, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMAPIKey != "" {
		t.Errorf("expected empty default API key, got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMModel != "" {
		t.Errorf("expected no model assumption, got %q", cfg.LLMModel)
	}
	if cfg.LLMProviderLabel != "agentrouter" {
		t.Errorf("expected default provider label, got %q", cfg.LLMProviderLabel)
	}
	if cfg.LLMContextBudget != 4000 {
		t.Errorf("expected default context budget 4000, got %d", cfg.LLMContextBudget)
	}
	if cfg.RetrievalMinScore != 0.6 {
		t.Errorf("expected default retrieval min score 0.6 (M4 frozen operating point), got %v", cfg.RetrievalMinScore)
	}
}

func TestLoadFromEnv_RetrievalMinScore(t *testing.T) {
	t.Setenv("RETRIEVAL_MIN_SCORE", "0.65")

	cfg := LoadFromEnv()

	if cfg.RetrievalMinScore != 0.65 {
		t.Errorf("expected configured retrieval min score 0.65, got %v", cfg.RetrievalMinScore)
	}
}

func TestLoadFromEnv_FusionPolicyName(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.FusionPolicyName != "balanced" {
		t.Errorf("expected default fusion policy balanced, got %q", cfg.FusionPolicyName)
	}

	t.Setenv("RETRIEVAL_FUSION_POLICY", "densebiased")
	if got := LoadFromEnv().FusionPolicyName; got != "densebiased" {
		t.Errorf("expected configured fusion policy densebiased, got %q", got)
	}
}

func TestLoadFromEnv_LLMSettings(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://llm.internal:9999/v1")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "some-router-model")
	t.Setenv("LLM_PROVIDER", "internal-gateway")
	t.Setenv("LLM_CONTEXT_BUDGET", "6000")

	cfg := LoadFromEnv()

	if cfg.LLMBaseURL != "http://llm.internal:9999/v1" {
		t.Errorf("expected configured base URL, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMAPIKey != "test-key" {
		t.Errorf("expected configured API key, got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMModel != "some-router-model" {
		t.Errorf("expected configured model, got %q", cfg.LLMModel)
	}
	if cfg.LLMProviderLabel != "internal-gateway" {
		t.Errorf("expected configured provider label, got %q", cfg.LLMProviderLabel)
	}
	if cfg.LLMContextBudget != 6000 {
		t.Errorf("expected configured context budget, got %d", cfg.LLMContextBudget)
	}
}

func TestBuildAnswerEngine_Wiring(t *testing.T) {
	ctx := context.Background()

	t.Run("wires the real OpenAI-compatible adapter, never the mock", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.LLMModel = ""
		engine := buildAnswerEngine(cfg, seededRetriever(ctx, t))

		// An unconfigured model must surface the real adapter's guard error,
		// proving the mock LLM is not silently installed by composition.
		_, err := engine.Answer(ctx, retrievalQuery("What is creativity?"))
		if err == nil || !strings.Contains(err.Error(), "model identifier is not configured") {
			t.Fatalf("expected real adapter model guard error, got %v", err)
		}

		p := buildLLMProvider(cfg)
		if _, ok := p.(*llmprovider.OpenAICompatibleProvider); !ok {
			t.Fatalf("expected OpenAICompatibleProvider, got %T", p)
		}
	})

	t.Run("no_evidence path works without any model configured", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.LLMModel = ""
		embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
		emptyRetriever := dense.NewDenseRetriever(embProvider, store.NewInMemoryVectorStore(), store.NewInMemoryContentStore())
		engine := buildAnswerEngine(cfg, emptyRetriever)

		ans, err := engine.Answer(ctx, retrievalQuery("No matches anywhere"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ans.Status != "no_evidence" {
			t.Errorf("expected no_evidence answer without LLM, got status %q", ans.Status)
		}
	})
}

func retrievalQuery(text string) seam.RetrievalQuery {
	return seam.RetrievalQuery{QueryText: text, TopK: 5}
}

func seededRetriever(ctx context.Context, t *testing.T) *dense.DenseRetriever {
	t.Helper()
	embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
	vecStore := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()
	seedTestChunk(ctx, t, embProvider, vecStore, contentStore)
	return dense.NewDenseRetriever(embProvider, vecStore, contentStore)
}

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
