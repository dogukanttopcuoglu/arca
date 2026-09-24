package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	graphstore "arca/internal/graph/store"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	llmprovider "arca/internal/llm/provider"
	qacontext "arca/internal/qa/context"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/rerank"
	"arca/internal/retrieval/seam"
)

func TestBuildVerificationPipeline(t *testing.T) {
	win := &qacontext.ContextWindow{
		Sources: []qacontext.SourceReference{
			{CitationKey: "[Ref 1]", DocumentID: "doc-1", Content: "source content"},
		},
	}

	t.Run("empty URL and key keep the pipeline structural-only (default-off)", func(t *testing.T) {
		rt := &Runtime{}
		p := buildVerificationPipeline(rt, DefaultConfig())
		if rt.verifier != nil {
			t.Fatal("no checker may be recorded when verification is off")
		}
		// The pipeline must behave byte-identical: Phase 1 alone decides.
		ans, err := p.Verify(context.Background(), "Grounded answer [Ref 1].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != "verified" {
			t.Errorf("status = %q, want verified", ans.Status)
		}
		if ans.Degraded {
			t.Error("must not degrade with no checker")
		}
		if ans.Semantic != nil {
			t.Errorf("semantic = %+v, want nil", ans.Semantic)
		}
	})

	t.Run("URL without a key stays default-off", func(t *testing.T) {
		rt := &Runtime{}
		cfg := DefaultConfig()
		cfg.VerifyJevURL = "http://localhost:3005"
		buildVerificationPipeline(rt, cfg)
		if rt.verifier != nil {
			t.Fatal("a checker must require both URL and API key")
		}
	})

	t.Run("configured URL and key create the checker and record it", func(t *testing.T) {
		rt := &Runtime{}
		cfg := DefaultConfig()
		cfg.VerifyJevURL = "http://localhost:3005"
		cfg.VerifyJevAPIKey = "test-key"
		buildVerificationPipeline(rt, cfg)
		if rt.verifier == nil {
			t.Fatal("checker must be recorded on the runtime when configured")
		}
	})
}

func TestDefaultConfig_VerifyJevOff(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.VerifyJevURL != "" {
		t.Fatalf("default Jev URL = %q, want empty (default-off)", cfg.VerifyJevURL)
	}
	if cfg.VerifyJevAPIKey != "" {
		t.Fatalf("default Jev API key = %q, want empty (default-off)", cfg.VerifyJevAPIKey)
	}
	if cfg.VerifyJevThreshold != 0.7 {
		t.Fatalf("default Jev threshold = %v, want 0.7", cfg.VerifyJevThreshold)
	}
	if cfg.VerifyJevTimeoutMS != 2000 {
		t.Fatalf("default Jev timeout = %dms, want 2000", cfg.VerifyJevTimeoutMS)
	}

	fromEnv := LoadFromEnv()
	if fromEnv.VerifyJevURL != "" || fromEnv.VerifyJevAPIKey != "" ||
		fromEnv.VerifyJevThreshold != 0.7 || fromEnv.VerifyJevTimeoutMS != 2000 {
		t.Fatalf("env defaults drifted: %+v", fromEnv)
	}
}

func TestLoadFromEnv_VerifyJev(t *testing.T) {
	t.Setenv("VERIFY_JEV_URL", "http://jev.internal:3005")
	t.Setenv("VERIFY_JEV_API_KEY", "test-key")
	t.Setenv("VERIFY_JEV_THRESHOLD", "0.8")
	t.Setenv("VERIFY_JEV_TIMEOUT_MS", "5000")

	cfg := LoadFromEnv()
	if cfg.VerifyJevURL != "http://jev.internal:3005" {
		t.Errorf("URL = %q", cfg.VerifyJevURL)
	}
	if cfg.VerifyJevAPIKey != "test-key" {
		t.Errorf("API key = %q", cfg.VerifyJevAPIKey)
	}
	if cfg.VerifyJevThreshold != 0.8 {
		t.Errorf("threshold = %v, want 0.8", cfg.VerifyJevThreshold)
	}
	if cfg.VerifyJevTimeoutMS != 5000 {
		t.Errorf("timeout = %dms, want 5000", cfg.VerifyJevTimeoutMS)
	}
}

func TestApplyEntityRerank(t *testing.T) {
	inner := &dense.DenseRetriever{}
	cfg := DefaultConfig()

	t.Run("empty URL keeps the fusion retriever unchanged (default-off)", func(t *testing.T) {
		rt := &Runtime{}
		got := applyEntityRerank(rt, inner, cfg)
		if got != inner {
			t.Fatalf("expected the unchanged fusion retriever, got %T", got)
		}
		if rt.reranker != nil {
			t.Fatal("no adapter may be recorded when reranking is off")
		}
	})

	t.Run("configured URL wraps the fusion retriever and records the adapter", func(t *testing.T) {
		rt := &Runtime{}
		cfg.RerankURL = "http://localhost:3003"
		cfg.RerankCandidateN = 50
		cfg.RerankTimeoutMS = 2000
		got := applyEntityRerank(rt, inner, cfg)
		wrapped, ok := got.(*rerank.RerankedRetriever)
		if !ok {
			t.Fatalf("expected RerankedRetriever wrapper, got %T", got)
		}
		if rt.reranker == nil {
			t.Fatal("adapter must be recorded for the stats block")
		}
		// The wrapper must request the E1-frozen candidate budget from the
		// inner retriever.
		requests := wrapped.RequestedBudget()
		if requests != 50 {
			t.Fatalf("candidate budget = %d, want 50", requests)
		}
	})
}

func TestDefaultConfig_RerankerOff(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RerankURL != "" {
		t.Fatalf("default rerank URL = %q, want empty (default-off)", cfg.RerankURL)
	}
	if cfg.RerankCandidateN != 50 {
		t.Fatalf("default candidate N = %d, want 50 (E1-frozen)", cfg.RerankCandidateN)
	}
	if cfg.RerankTimeoutMS != 2000 {
		t.Fatalf("default timeout = %dms, want 2000", cfg.RerankTimeoutMS)
	}

	fromEnv := LoadFromEnv()
	if fromEnv.RerankURL != "" || fromEnv.RerankCandidateN != 50 || fromEnv.RerankTimeoutMS != 2000 {
		t.Fatalf("env defaults drifted: %+v", fromEnv)
	}
}

func TestRenderRerankerStats(t *testing.T) {
	t.Run("nil adapter renders Enabled false", func(t *testing.T) {
		out := renderRerankerStats(nil)
		if !strings.Contains(out, "Enabled: false") {
			t.Fatalf("expected disabled block, got:\n%s", out)
		}
	})

	t.Run("adapter counters render the full block", func(t *testing.T) {
		r := rerank.NewHTTPReranker("http://localhost:3003", time.Second)
		out := renderRerankerStats(r)
		for _, want := range []string{"Enabled: true", "Requests: 0", "Failures: 0", "AvgLatencyMs: 0", "Degraded: false"} {
			if !strings.Contains(out, want) {
				t.Fatalf("block missing %q:\n%s", want, out)
			}
		}
	})
}

func TestAppRunAsk_RerankerBlock(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured runtime renders Enabled false", func(t *testing.T) {
		app := newTestApp(ctx, t, "Grounded answer [Ref 1].")
		out, err := app.RunAsk(ctx, "What is creativity?", "")
		if err != nil {
			t.Fatalf("RunAsk: %v", err)
		}
		if !strings.Contains(out, "Reranker:") || !strings.Contains(out, "Enabled: false") {
			t.Fatalf("expected disabled Reranker block, got:\n%s", out)
		}
	})

	t.Run("configured runtime renders the enabled block", func(t *testing.T) {
		app := newTestApp(ctx, t, "Grounded answer [Ref 1].")
		// The adapter is composed by buildAnswerEngine (ticket 03 wiring);
		// the live invocation and counters are verified end-to-end in the
		// M10 activation gate.
		app.runtime = &Runtime{reranker: rerank.NewHTTPReranker("http://localhost:3003", time.Second)}
		out, err := app.RunAsk(ctx, "What is creativity?", "")
		if err != nil {
			t.Fatalf("RunAsk: %v", err)
		}
		for _, want := range []string{"Enabled: true", "Requests: 0", "Failures: 0", "AvgLatencyMs: 0", "Degraded: false"} {
			if !strings.Contains(out, want) {
				t.Fatalf("expected live Reranker block with %q, got:\n%s", want, out)
			}
		}
	})
}

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
	if cfg.ComparisonTopK != 8 {
		t.Errorf("expected default comparison TopK 8 (M6 calibrated evidence budget), got %d", cfg.ComparisonTopK)
	}
	if cfg.RetrievalGraphWeight != 1.0 {
		t.Errorf("expected default graph weight 1.0 (M7 calibrated fusion weight), got %v", cfg.RetrievalGraphWeight)
	}
}

func TestLoadFromEnv_RetrievalGraphWeight(t *testing.T) {
	t.Setenv("RETRIEVAL_GRAPH_WEIGHT", "0")
	cfg := LoadFromEnv()
	if cfg.RetrievalGraphWeight != 0 {
		t.Errorf("expected configured graph weight 0, got %v", cfg.RetrievalGraphWeight)
	}
}

func TestLoadFromEnv_RetrievalMinScore(t *testing.T) {
	t.Setenv("RETRIEVAL_MIN_SCORE", "0.65")

	cfg := LoadFromEnv()

	if cfg.RetrievalMinScore != 0.65 {
		t.Errorf("expected configured retrieval min score 0.65, got %v", cfg.RetrievalMinScore)
	}
}

func TestLoadFromEnv_ComparisonTopK(t *testing.T) {
	t.Setenv("RETRIEVAL_COMPARISON_TOP_K", "10")
	cfg := LoadFromEnv()
	if cfg.ComparisonTopK != 10 {
		t.Errorf("expected configured comparison TopK 10, got %d", cfg.ComparisonTopK)
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
		cfg.RetrievalGraphWeight = 0 // keep the gate closed for this wiring test
		rt, err := NewRuntime(cfg)
		if err != nil {
			t.Fatalf("failed to construct runtime: %v", err)
		}
		engine := buildAnswerEngine(rt, seededRetriever(ctx, t))

		// An unconfigured model must surface the real adapter's guard error,
		// proving the mock LLM is not silently installed by composition.
		_, err = engine.Answer(ctx, retrievalQuery("What is creativity?"))
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
		cfg.RetrievalGraphWeight = 0
		rt, err := NewRuntime(cfg)
		if err != nil {
			t.Fatalf("failed to construct runtime: %v", err)
		}
		embProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
		emptyRetriever := dense.NewDenseRetriever(embProvider, store.NewInMemoryVectorStore(), store.NewInMemoryContentStore())
		engine := buildAnswerEngine(rt, emptyRetriever)

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

func TestGraphStoreFor(t *testing.T) {
	t.Run("qdrant config selects the Qdrant graph store on the REST port", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.VectorStoreType = VectorStoreQdrant
		cfg.VectorStoreURL = "http://localhost:6334"
		gs, err := graphStoreFor(cfg)
		if err != nil {
			t.Fatalf("graphStoreFor: %v", err)
		}
		if _, ok := gs.(*graphstore.QdrantGraphStore); !ok {
			t.Fatalf("store type = %T, want *QdrantGraphStore", gs)
		}
	})
	t.Run("non-qdrant config selects the in-memory store", func(t *testing.T) {
		gs, err := graphStoreFor(DefaultConfig())
		if err != nil {
			t.Fatalf("graphStoreFor: %v", err)
		}
		if _, ok := gs.(*graphstore.InMemoryGraphStore); !ok {
			t.Fatalf("store type = %T, want *InMemoryGraphStore", gs)
		}
	})
}
