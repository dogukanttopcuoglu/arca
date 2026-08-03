package cli

import (
	"fmt"
	"time"

	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
	"arca/internal/pdfinspector/assets"
	"arca/internal/pdfinspector/chunking"
	"arca/internal/pdfinspector/config"
	"arca/internal/pdfinspector/diagnostics"
	"arca/internal/pdfinspector/firecrawl"
	"arca/internal/pdfinspector/inspector"
	"arca/internal/pdfinspector/semantic"
	"arca/internal/retrieval/dense"

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
	// VectorStoreURL is the connection endpoint for the vector store (Qdrant REST URL).
	VectorStoreURL string `mapstructure:"QDRANT_URL"`
	// QdrantCollection is the Qdrant collection name to use.
	QdrantCollection string `mapstructure:"QDRANT_COLLECTION"`
	// HTTPTimeout is the client timeout for external service calls.
	HTTPTimeout time.Duration `mapstructure:"HTTP_TIMEOUT"`
}

// DefaultConfig returns the M1 defaults: real Firecrawl, mock embedding provider,
// in-memory vector store. Real adapters slot in as they land (Qdrant/Nomic decisions).
func DefaultConfig() Config {
	return Config{
		FirecrawlBaseURL:      "http://localhost:3002",
		EmbeddingProviderType: EmbeddingProviderMock,
		EmbeddingModel:        "mock-model-v1",
		OllamaBaseURL:         "http://localhost:11434",
		VectorStoreType:       VectorStoreInMemory,
		VectorStoreURL:        "http://localhost:6333",
		QdrantCollection:      "arca_chunks",
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
	v.SetDefault("HTTP_TIMEOUT", base.HTTPTimeout)

	return Config{
		FirecrawlBaseURL:      v.GetString("FIRECRAWL_BASE_URL"),
		EmbeddingProviderType: EmbeddingProviderType(v.GetString("EMBEDDING_PROVIDER")),
		EmbeddingModel:        v.GetString("EMBEDDING_MODEL"),
		OllamaBaseURL:         v.GetString("OLLAMA_URL"),
		VectorStoreType:       VectorStoreType(v.GetString("VECTOR_STORE")),
		VectorStoreURL:        v.GetString("QDRANT_URL"),
		QdrantCollection:      v.GetString("QDRANT_COLLECTION"),
		HTTPTimeout:           v.GetDuration("HTTP_TIMEOUT"),
	}
}

// Runtime is the composition root that wires the full inspect -> index -> retrieve pipeline.
// It is the M1 production entrypoint; tests may construct it directly with mock adapters.
type Runtime struct {
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
	indexingWorker := worker.NewIndexingWorker(embProvider, vecStore, contentStore)
	denseRetriever := dense.NewDenseRetriever(embProvider, vecStore, contentStore)

	rt := &Runtime{
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
		// Real Qdrant adapter lands with the vector store decision; M1 defaults to in-memory.
		return nil, fmt.Errorf("qdrant vector store adapter not yet implemented")
	default:
		return store.NewInMemoryVectorStore(), nil
	}
}
