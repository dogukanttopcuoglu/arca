package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	graphretriever "arca/internal/graph/retriever"
	graphstore "arca/internal/graph/store"
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
	"arca/internal/retrieval/graphfusion"
	"arca/internal/retrieval/hybrid"
	retrievalseam "arca/internal/retrieval/seam"
	retrievalsparse "arca/internal/retrieval/sparse"

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
	// RetrievalMode selects the retriever used by arc ask: dense (default),
	// sparse, or hybrid. Sparse and hybrid require SparseIndex.
	RetrievalMode retrievalseam.RetrievalMode `mapstructure:"RETRIEVAL_MODE"`
	// FusionPolicyName selects a frozen, calibrated fusion policy for hybrid
	// retrieval (balanced | densebiased). Resolution happens in composition;
	// the name is never interpreted at retrieval time.
	FusionPolicyName string `mapstructure:"RETRIEVAL_FUSION_POLICY"`
	// ComparisonTopK is the M6 evidence budget for comparison queries
	// (ADR-0037): the orchestrator raises TopK for decomposed comparison
	// sub-queries and trims the merge to the same value. Zero keeps the
	// caller's TopK. Calibrated on Gold Set v2: 8 (comparison false
	// abstentions 4/4 -> 2/4, abstention precision 0.615 -> 0.727, recall
	// flat; TopK 10 rejected — upstream 429 rate limiting invalidated it).
	ComparisonTopK int `mapstructure:"RETRIEVAL_COMPARISON_TOP_K"`
	// RetrievalGraphWeight is the M7 graph fusion weight (ADR-0042): >0
	// activates the graph gate for entity queries through the injected
	// GraphFusionRetriever. Frozen at 1.0 by the Gold Set v3 calibration
	// (ADR-0041); 0 keeps the graph gate closed.
	RetrievalGraphWeight float64 `mapstructure:"RETRIEVAL_GRAPH_WEIGHT"`
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
		RetrievalMinScore:     0.6, // frozen M4 calibrated operating point (ADR-0036)
		SparseIndex:           false,
		RetrievalMode:         retrievalseam.RetrievalDense,
		FusionPolicyName:      "balanced",
		ComparisonTopK:        8,   // M6 calibrated evidence budget (ADR-0037)
		RetrievalGraphWeight:  1.0, // M7 calibrated graph fusion weight (ADR-0041)
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
	v.SetDefault("RETRIEVAL_MODE", base.RetrievalMode.String())
	v.SetDefault("RETRIEVAL_FUSION_POLICY", base.FusionPolicyName)
	v.SetDefault("RETRIEVAL_COMPARISON_TOP_K", base.ComparisonTopK)
	v.SetDefault("RETRIEVAL_GRAPH_WEIGHT", base.RetrievalGraphWeight)
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
		RetrievalMode:         parseRetrievalMode(v.GetString("RETRIEVAL_MODE")),
		FusionPolicyName:      v.GetString("RETRIEVAL_FUSION_POLICY"),
		ComparisonTopK:        v.GetInt("RETRIEVAL_COMPARISON_TOP_K"),
		RetrievalGraphWeight:  v.GetFloat64("RETRIEVAL_GRAPH_WEIGHT"),
		HTTPTimeout:           v.GetDuration("HTTP_TIMEOUT"),
	}
}

// parseRetrievalMode maps a configured mode string to the retrieval mode
// enum, defaulting to dense for unknown values.
func parseRetrievalMode(s string) retrievalseam.RetrievalMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sparse":
		return retrievalseam.RetrievalSparse
	case "hybrid":
		return retrievalseam.RetrievalHybrid
	default:
		return retrievalseam.RetrievalDense
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

	// Sparse retrieval components are built lazily: the query encoder needs
	// the indexed corpus, which does not exist yet on a fresh collection.
	sparseOnce      sync.Once
	sparseEncoder   sparse.SparseEncoder
	sparseErr       error
	sparseRetriever retrievalseam.Retriever
	hybridRetriever retrievalseam.Retriever

	// Graph retrieval components (M7, ADR-0038): the entity-only graph store
	// and the entity-overlap retriever, built lazily.
	graphOnce      sync.Once
	graphErr       error
	graphRetriever retrievalseam.Retriever

	// Graph fusion retriever (ADR-0042): dense + graph streams fused with
	// the frozen weight, built lazily for the orchestration gate.
	fusionOnce      sync.Once
	fusionErr       error
	fusionRetriever retrievalseam.Retriever
}

// GraphRetriever builds the entity-only graph retriever (ADR-0038/0039):
// QdrantGraphStore against the node collection when the vector store is
// Qdrant, otherwise the in-memory graph store. Chunk content is resolved
// from the vector store payload (production: the process-local ContentStore
// is empty). A failed build is not cached.
func (r *Runtime) GraphRetriever() (retrievalseam.Retriever, error) {
	r.graphOnce.Do(func() {
		var gs graphstore.GraphStore
		if r.cfg.VectorStoreType == VectorStoreQdrant {
			gs, r.graphErr = graphstore.NewQdrantGraphStore(graphRestBaseURL(r.cfg), "")
		} else {
			gs = graphstore.NewInMemoryGraphStore()
		}
		if r.graphErr == nil {
			r.graphRetriever = graphretriever.NewGraphRetriever(gs, r.contentStore,
				graphretriever.WithVectorStore(r.vectorStore))
		}
	})
	if r.graphErr != nil {
		r.graphOnce = sync.Once{}
	}
	return r.graphRetriever, r.graphErr
}

// GraphFusionRetriever builds the graph fusion retriever (ADR-0042): the
// dense stream fused with the entity graph stream at the frozen calibration
// weight (RETRIEVAL_GRAPH_WEIGHT). A failed build is not cached.
func (r *Runtime) GraphFusionRetriever() (retrievalseam.Retriever, error) {
	r.fusionOnce.Do(func() {
		graphRet, err := r.GraphRetriever()
		if err != nil {
			r.fusionErr = err
			return
		}
		r.fusionRetriever = graphfusion.NewGraphFusionRetriever(
			r.denseRetriever,
			graphRet,
			graphfusion.GraphFusionConfig{
				DenseWeight: graphfusion.DefaultDenseWeight,
				GraphWeight: r.cfg.RetrievalGraphWeight,
			},
		)
	})
	if r.fusionErr != nil {
		r.fusionOnce = sync.Once{}
	}
	return r.fusionRetriever, r.fusionErr
}

// graphRestBaseURL derives the Qdrant REST base URL from the configured
// vector store URL. The config points at the gRPC port (6334); the graph
// store speaks REST (6333) because gRPC upsert rejects vectorless points.
// The default gRPC port maps to the default REST port; non-default ports are
// preserved verbatim.
func graphRestBaseURL(cfg Config) string {
	host := strings.TrimPrefix(strings.TrimPrefix(cfg.VectorStoreURL, "http://"), "https://")
	if h, port, ok := strings.Cut(host, ":"); ok {
		if port == "6334" {
			return "http://" + h + ":6333"
		}
		return "http://" + host
	}
	return "http://" + host + ":6333"
}

// sparseQueryEncoder builds the corpus-bound query encoder. A failed build is
// not cached: on a fresh collection the corpus does not exist yet, so the
// next call after indexing retries.
func (r *Runtime) sparseQueryEncoder() (sparse.SparseEncoder, error) {
	r.sparseOnce.Do(func() {
		provider := sparse.NewBM25EncoderProvider(corpusSource{store: r.vectorStore})
		r.sparseEncoder, r.sparseErr = provider.EncoderForCorpus(context.Background())
	})
	if r.sparseErr != nil {
		r.sparseOnce = sync.Once{}
	}
	return r.sparseEncoder, r.sparseErr
}

// RetrieverForMode returns the retriever for the given retrieval mode. Sparse
// and hybrid require an indexed corpus (the query encoder is corpus-bound).
func (r *Runtime) RetrieverForMode(mode retrievalseam.RetrievalMode) (retrievalseam.Retriever, error) {
	switch mode {
	case retrievalseam.RetrievalDense:
		return r.denseRetriever, nil
	case retrievalseam.RetrievalSparse:
		if r.sparseRetriever == nil {
			enc, err := r.sparseQueryEncoder()
			if err != nil {
				return nil, fmt.Errorf("sparse retrieval requires an indexed corpus: %w", err)
			}
			r.sparseRetriever = retrievalsparse.NewSparseRetriever(enc, r.vectorStore, r.contentStore)
		}
		return r.sparseRetriever, nil
	case retrievalseam.RetrievalHybrid:
		if r.hybridRetriever == nil {
			sparseRet, err := r.RetrieverForMode(retrievalseam.RetrievalSparse)
			if err != nil {
				return nil, err
			}
			policy, err := hybrid.PolicyByName(r.cfg.FusionPolicyName)
			if err != nil {
				return nil, fmt.Errorf("invalid fusion policy: %w", err)
			}
			r.hybridRetriever = hybrid.NewHybridRetriever(r.denseRetriever, sparseRet, hybrid.WithFusionPolicy(policy))
		}
		return r.hybridRetriever, nil
	default:
		return nil, fmt.Errorf("unknown retrieval mode %v", mode)
	}
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
// the default verification pipeline, the real EvidenceGate (ADR-0030), the
// benchmark-calibrated retrieval runtime config, and the graph fusion
// retriever when the frozen graph weight is active (ADR-0042). Retrieval stays
// on the provided seam.
func buildAnswerEngine(rt *Runtime, retriever retrievalseam.Retriever) *qa.AnswerEngine {
	cfg := rt.cfg
	opts := []qa.AnswerEngineOption{
		qa.WithRetrievalRuntimeConfig(qa.RetrievalRuntimeConfig{
			ComparisonTopK: cfg.ComparisonTopK,
			GraphWeight:    cfg.RetrievalGraphWeight,
		}),
	}
	if cfg.RetrievalGraphWeight > 0 {
		fusionRet, err := rt.GraphFusionRetriever()
		if err != nil {
			// The gate must never fail silently: a graph build failure with a
			// positive configured weight is surfaced (ADR-0042 composition).
			fmt.Fprintf(os.Stderr, "warning: graph fusion unavailable (RETRIEVAL_GRAPH_WEIGHT=%.1f): %v; falling back to %s\n",
				cfg.RetrievalGraphWeight, err, cfg.RetrievalMode)
		} else {
			opts = append(opts, qa.WithGraphRetriever(fusionRet))
		}
	}
	return qa.NewAnswerEngine(
		nil,
		retriever,
		qacontext.NewDefaultContextBuilder(nil, cfg.LLMContextBudget),
		qaprompt.NewRAGPromptBuilder(),
		buildLLMProvider(cfg),
		qaverification.NewDefaultVerificationPipeline(),
		qa.NewLLMEvidenceGate(buildLLMProvider(cfg)),
		opts...,
	)
}
