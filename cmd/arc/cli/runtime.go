package cli

import (
	"context"
	"strings"
	"time"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
	llmprovider "arca/internal/llm/provider"
	"arca/internal/pdfinspector/assets"
	"arca/internal/pdfinspector/chunking"
	"arca/internal/pdfinspector/config"
	"arca/internal/pdfinspector/diagnostics"
	"arca/internal/pdfinspector/firecrawl"
	"arca/internal/pdfinspector/inspector"
	"arca/internal/pdfinspector/semantic"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
	qaverification "arca/internal/qa/verification"
	"arca/internal/retrieval/dense"
	"arca/internal/retrieval/seam"

	"github.com/spf13/viper"
)

// EmbeddingProviderType selects the embedding provider implementation for M1.
type EmbeddingProviderType string

const (
	EmbeddingProviderMock  EmbeddingProviderType = "mock"
	EmbeddingProviderNomic EmbeddingProviderType = "nomic"
)

// VectorStoreType selects the vector store implementation for M1.
type VectorStoreType string

const (
	VectorStoreInMemory VectorStoreType = "inmemory"
	VectorStoreQdrant   VectorStoreType = "qdrant"
)

// Config defines runtime configuration for the ARC CLI composition root.
type Config struct {
	// FirecrawlBaseURL points at the Firecrawl PDF extraction service.
	FirecrawlBaseURL string `mapstructure:"FIRECRAWL_BASE_URL"`
	// EmbeddingProviderType selects the embedding provider adapter.
	EmbeddingProviderType EmbeddingProviderType `mapstructure:"EMBEDDING_PROVIDER"`
	// EmbeddingModel is the pinned embedding model identifier. For real providers
	// this must be a stable tag (never :latest) so IndexSignature stays stable.
	EmbeddingModel string `mapstructure:"EMBEDDING_MODEL"`
	// OllamaBaseURL is the local Ollama runtime endpoint for nomic embedding.
	OllamaBaseURL string `mapstructure:"OLLAMA_URL"`
	// VectorStoreType selects the vector store adapter.
	VectorStoreType VectorStoreType `mapstructure:"VECTOR_STORE"`
	// VectorStoreURL is the connection endpoint for the vector store (Qdrant gRPC endpoint).
	VectorStoreURL string `mapstructure:"QDRANT_URL"`
	// QdrantCollection is the Qdrant collection name to use.
	QdrantCollection string `mapstructure:"QDRANT_COLLECTION"`
	// LLMBaseURL is the OpenAI-compatible gateway root (includes any /v1 prefix).
	LLMBaseURL string `mapstructure:"LLM_BASE_URL"`
	// LLMAPIKey is the bearer credential for the gateway (empty for keyless endpoints).
	LLMAPIKey string `mapstructure:"LLM_API_KEY"`
	// LLMModel is the model identifier, forwarded verbatim; never assumed.
	LLMModel string `mapstructure:"LLM_MODEL"`
	// LLMProviderLabel is an observability-only label for AnswerMetadata; it
	// never affects adapter or pipeline behavior.
	LLMProviderLabel string `mapstructure:"LLM_PROVIDER"`
	// LLMContextBudget is the composition-owned ContextWindow token budget.
	LLMContextBudget int `mapstructure:"LLM_CONTEXT_BUDGET"`
	// RetrievalMinScore is the minimum relevance score for retrieved chunks.
	// Zero disables the threshold (retrieval returns top-k neighbors). With a
	// threshold, queries whose results all fall below it abstain (no_evidence).
	RetrievalMinScore float32 `mapstructure:"RETRIEVAL_MIN_SCORE"`
	// SparseIndex enables sparse vectors: collections are created with a named
	// sparse field and indexing produces sparse vectors. Explicit opt-in;
	// existing dense-only collections remain untouched.
	SparseIndex bool `mapstructure:"SPARSE_INDEX"`
	// HTTPTimeout is the client timeout for external service calls.
	HTTPTimeout time.Duration `mapstructure:"HTTP_TIMEOUT"`
}

// DefaultConfig returns the M1 defaults: real Firecrawl, mock embedding provider,
// in-memory vector store, and the ADR-0026 OpenAI-compatible generation gateway.
// Real adapters slot in as they land (Qdrant/Nomic decisions).
func DefaultConfig() Config {
	return Config{
		FirecrawlBaseURL:      "http://localhost:3002",
		EmbeddingProviderType: EmbeddingProviderMock,
		EmbeddingModel:        "mock-model-v1",
		OllamaBaseURL:         "http://localhost:11434",
		VectorStoreType:       VectorStoreInMemory,
		VectorStoreURL:        "http://localhost:6334", // gRPC port; the adapter does not speak REST
		QdrantCollection:      "arca_chunks",
		LLMBaseURL:            "https://agentrouter.org/v1",
		LLMProviderLabel:      "agentrouter",
		LLMContextBudget:      4000,
		RetrievalMinScore:     0,
		SparseIndex:           false,
		HTTPTimeout:           30 * time.Second,
	}
}

// LoadFromEnv loads the runtime configuration using Viper from environment variables,
// following the same conventions as internal/pdfinspector/config (AutomaticEnv, defaults, env names).
func LoadFromEnv() Config {
	base := DefaultConfig()

	v := viper.New()
	v.AutomaticEnv()

	v.SetDefault("FIRECRAWL_BASE_URL", base.FirecrawlBaseURL)
	v.SetDefault("EMBEDDING_PROVIDER", string(base.EmbeddingProviderType))
	v.SetDefault("EMBEDDING_MODEL", base.EmbeddingModel)
	v.SetDefault("OLLAMA_URL", base.OllamaBaseURL)
	v.SetDefault("VECTOR_STORE", string(base.VectorStoreType))
	v.SetDefault("QDRANT_URL", base.VectorStoreURL)
	v.SetDefault("QDRANT_COLLECTION", base.QdrantCollection)
	v.SetDefault("LLM_BASE_URL", base.LLMBaseURL)
	v.SetDefault("LLM_API_KEY", base.LLMAPIKey)
	v.SetDefault("LLM_MODEL", base.LLMModel)
	v.SetDefault("LLM_PROVIDER", base.LLMProviderLabel)
	v.SetDefault("LLM_CONTEXT_BUDGET", base.LLMContextBudget)
	v.SetDefault("RETRIEVAL_MIN_SCORE", base.RetrievalMinScore)
	v.SetDefault("SPARSE_INDEX", base.SparseIndex)
	v.SetDefault("HTTP_TIMEOUT", base.HTTPTimeout)

	return Config{
		FirecrawlBaseURL:      v.GetString("FIRECRAWL_BASE_URL"),
		EmbeddingProviderType: EmbeddingProviderType(v.GetString("EMBEDDING_PROVIDER")),
		EmbeddingModel:        v.GetString("EMBEDDING_MODEL"),
		OllamaBaseURL:         v.GetString("OLLAMA_URL"),
		VectorStoreType:       VectorStoreType(v.GetString("VECTOR_STORE")),
		VectorStoreURL:        v.GetString("QDRANT_URL"),
		QdrantCollection:      v.GetString("QDRANT_COLLECTION"),
		LLMBaseURL:            v.GetString("LLM_BASE_URL"),
		LLMAPIKey:             v.GetString("LLM_API_KEY"),
		LLMModel:              v.GetString("LLM_MODEL"),
		LLMProviderLabel:      v.GetString("LLM_PROVIDER"),
		LLMContextBudget:      v.GetInt("LLM_CONTEXT_BUDGET"),
		RetrievalMinScore:     float32(v.GetFloat64("RETRIEVAL_MIN_SCORE")),
		SparseIndex:           v.GetBool("SPARSE_INDEX"),
		HTTPTimeout:           v.GetDuration("HTTP_TIMEOUT"),
	}
}

// Runtime is the composition root that wires the full inspect -> index -> retrieve pipeline.
// It is the M1 production entrypoint; tests may construct it directly with mock adapters.
type Runtime struct {
	cfg               Config
	inspector         inspector.Inspector
	embeddingProvider provider.EmbeddingProvider
	vectorStore       store.VectorStore
	inMemoryStore     *store.InMemoryVectorStore
	contentStore      store.ContentStore
	indexingWorker    *worker.IndexingWorker
	denseRetriever    *dense.DenseRetriever
}

// NewRuntime constructs the composition root from Config.
func NewRuntime(cfg Config) (*Runtime, error) {
	insp := buildInspector(cfg)

	embProvider, err := buildEmbeddingProvider(cfg)
	if err != nil {
		return nil, err
	}

	vecStore, err := buildVectorStore(cfg)
	if err != nil {
		return nil, err
	}

	contentStore := store.NewInMemoryContentStore()
	indexingOpts := []worker.IndexingWorkerOption{}
	if cfg.SparseIndex {
		sparseProvider := sparse.NewBM25EncoderProvider(corpusSource{store: vecStore})
		indexingOpts = append(indexingOpts, worker.WithSparseEncoderProvider(sparseProvider))
	}
	indexingWorker := worker.NewIndexingWorker(embProvider, vecStore, contentStore, indexingOpts...)
	denseRetriever := dense.NewDenseRetriever(embProvider, vecStore, contentStore)

	rt := &Runtime{
		cfg:               cfg,
		inspector:         insp,
		embeddingProvider: embProvider,
		vectorStore:       vecStore,
		contentStore:      contentStore,
		indexingWorker:    indexingWorker,
		denseRetriever:    denseRetriever,
	}
	if mem, ok := vecStore.(*store.InMemoryVectorStore); ok {
		rt.inMemoryStore = mem
	}
	return rt, nil
}

// StoredPoints returns the number of vector points persisted for diagnostics.
// Returns 0 for non-in-memory stores.
func (r *Runtime) StoredPoints() int {
	if r.inMemoryStore == nil {
		return 0
	}
	return r.inMemoryStore.Points()
}

func buildInspector(cfg Config) inspector.Inspector {
	client := firecrawl.NewHTTPClient(cfg.FirecrawlBaseURL, firecrawl.WithTimeout(cfg.HTTPTimeout))
	processor := semantic.NewProcessor()
	chunker := chunking.NewEngine()
	extractor := assets.NewExtractor()
	aggregator := diagnostics.NewAggregator()

	return inspector.NewPDFInspector(config.LoadFromEnv(), client, processor, chunker, extractor, aggregator)
}

func buildEmbeddingProvider(cfg Config) (provider.EmbeddingProvider, error) {
	switch cfg.EmbeddingProviderType {
	case EmbeddingProviderNomic:
		// Real Nomic adapter backed by the local Ollama runtime. The model must be a
		// pinned tag so IndexSignature stays stable across re-indexing.
		return provider.NewOllamaEmbeddingProvider(cfg.OllamaBaseURL, cfg.EmbeddingModel), nil
	default:
		return provider.NewMockEmbeddingProvider("mock-provider", cfg.EmbeddingModel, 1536), nil
	}
}

func buildVectorStore(cfg Config) (store.VectorStore, error) {
	switch cfg.VectorStoreType {
	case VectorStoreQdrant:
		// Real Qdrant adapter backed by the official Go client. Vector storage only;
		// chunk content lives in the ContentStore seam, not the vector payload.
		host := strings.TrimPrefix(strings.TrimPrefix(cfg.VectorStoreURL, "http://"), "https://")
		opts := []store.QdrantOption{}
		if cfg.SparseIndex {
			opts = append(opts, store.WithSparseVectors())
		}
		qstore, err := store.NewQdrantVectorStore(host, cfg.QdrantCollection, opts...)
		if err != nil {
			return nil, err
		}
		return qstore, nil
	default:
		return store.NewInMemoryVectorStore(), nil
	}
}

// corpusSource implements sparse.CorpusSource over the real vector store,
// returning the markdown content of every indexed chunk.
type corpusSource struct {
	store store.VectorStore
}

// CorpusTexts returns the content of every indexed chunk in the collection.
func (s corpusSource) CorpusTexts(ctx context.Context) ([]string, error) {
	points, err := s.store.ListPoints(ctx, indexingmodel.MetadataFilter{})
	if err != nil {
		return nil, err
	}
	texts := make([]string, 0, len(points))
	for _, p := range points {
		if p.ContentMarkdown != "" {
			texts = append(texts, p.ContentMarkdown)
		}
	}
	return texts, nil
}

// buildLLMProvider constructs the provider-neutral OpenAI-compatible LLM
// adapter from configuration. The provider label is observability-only and
// never affects adapter behavior.
func buildLLMProvider(cfg Config) llmprovider.LLMProvider {
	return llmprovider.NewOpenAICompatibleProvider(
		cfg.LLMBaseURL,
		cfg.LLMAPIKey,
		cfg.LLMModel,
		cfg.LLMProviderLabel,
	)
}

// buildAnswerEngine wires the real AnswerEngine seams: ContextBuilder with the
// configured budget, the RAG PromptBuilder, the OpenAI-compatible LLM adapter,
// and the default verification pipeline. Retrieval stays on the provided seam.
func buildAnswerEngine(cfg Config, retriever seam.Retriever) *qa.AnswerEngine {
	return qa.NewAnswerEngine(
		nil,
		retriever,
		qacontext.NewDefaultContextBuilder(nil, cfg.LLMContextBudget),
		qaprompt.NewRAGPromptBuilder(),
		buildLLMProvider(cfg),
		qaverification.NewDefaultVerificationPipeline(),
	)
}
