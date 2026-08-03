package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

// Ollama endpoints used by the adapter.
const (
	ollamaEmbeddingsEndpoint = "/api/embeddings" // single prompt embedding
	ollamaEmbedEndpoint      = "/api/embed"      // batch input embedding
	ollamaTagsEndpoint       = "/api/tags"       // health/model listing
)

// Nomic retrieval prefixes required for correct retrieval quality. Documents are
// embedded with the search_document prefix and queries with search_query; the
// adapter encapsulates this so callers never see model-specific prefix behavior.
const (
	ollamaDocPrefix   = "search_document: "
	ollamaQueryPrefix = "search_query: "
)

// Embedding version pinned for IndexSignature stability across re-indexing.
const ollamaEmbeddingVersion = "1.0.0"

// OllamaEmbeddingProvider is an EmbeddingProvider adapter backed by a local Ollama
// runtime (nomic-embed-text) exposing its HTTP API. It is the M1 local embedding
// backend: no cloud dependency, deterministic vector dimension (768), and batch
// embedding via /api/embed.
type OllamaEmbeddingProvider struct {
	baseURL   string
	model     string
	dimension int
	client    *fasthttp.Client
	timeout   time.Duration
}

// OllamaOption configures an OllamaEmbeddingProvider instance.
type OllamaOption func(*OllamaEmbeddingProvider)

// WithOllamaBaseURL overrides the default Ollama base URL.
func WithOllamaBaseURL(baseURL string) OllamaOption {
	return func(o *OllamaEmbeddingProvider) {
		if baseURL != "" {
			o.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithOllamaHTTPClient overrides the fasthttp client used for requests.
func WithOllamaHTTPClient(client *fasthttp.Client) OllamaOption {
	return func(o *OllamaEmbeddingProvider) {
		if client != nil {
			o.client = client
		}
	}
}

// WithOllamaTimeout overrides the request timeout.
func WithOllamaTimeout(timeout time.Duration) OllamaOption {
	return func(o *OllamaEmbeddingProvider) {
		if timeout > 0 {
			o.timeout = timeout
		}
	}
}

// WithOllamaDimension overrides the expected embedding vector dimension.
func WithOllamaDimension(dimension int) OllamaOption {
	return func(o *OllamaEmbeddingProvider) {
		if dimension > 0 {
			o.dimension = dimension
		}
	}
}

// NewOllamaEmbeddingProvider constructs an OllamaEmbeddingProvider targeting the
// local Ollama runtime. The model should be a pinned tag (e.g. nomic-embed-text:latest)
// so IndexSignature remains stable across re-indexing.
func NewOllamaEmbeddingProvider(baseURL, model string, opts ...OllamaOption) *OllamaEmbeddingProvider {
	o := &OllamaEmbeddingProvider{
		baseURL:   "http://localhost:11434",
		model:     "nomic-embed-text:latest",
		dimension: 768,
		client:    &fasthttp.Client{},
		timeout:   60 * time.Second,
	}

	if baseURL != "" {
		o.baseURL = strings.TrimRight(baseURL, "/")
	}
	if model != "" {
		o.model = model
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// Provider returns the canonical provider identifier.
func (o *OllamaEmbeddingProvider) Provider() string {
	return "Ollama"
}

// Model returns the embedding model identifier.
func (o *OllamaEmbeddingProvider) Model() string {
	return o.model
}

// Capabilities returns provider operational bounds.
func (o *OllamaEmbeddingProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Dimension:      o.dimension,
		MaxBatchSize:   100,
		MaxInputTokens: 8192, // nomic-embed-text context length
		SupportsBatch:  true,
	}
}

// Health verifies Ollama connectivity via the model tags endpoint.
func (o *OllamaEmbeddingProvider) Health(ctx context.Context) error {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}()

	req.SetRequestURI(o.baseURL + ollamaTagsEndpoint)
	req.Header.SetMethod(fasthttp.MethodGet)

	if err := o.client.DoTimeout(req, resp, o.timeout); err != nil {
		return fmt.Errorf("ollama health check failed: %w", err)
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return fmt.Errorf("ollama health check returned status %d", resp.StatusCode())
	}
	return nil
}

// EmbedDocuments produces dense vector embeddings for a batch of document texts.
// Documents are prefixed with "search_document: " per Nomic retrieval guidance.
func (o *OllamaEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) (*EmbeddingResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(texts) == 0 {
		return &EmbeddingResult{
			Vectors:  [][]float32{},
			Provider: o.Provider(),
			Model:    o.model,
			Version:  ollamaEmbeddingVersion,
		}, nil
	}

	prefixed := make([]string, len(texts))
	for i, t := range texts {
		prefixed[i] = ollamaDocPrefix + t
	}

	body, err := json.Marshal(map[string]interface{}{
		"model": o.model,
		"input": prefixed,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama embed request: %w", err)
	}

	respBody, err := o.doPost(ctx, ollamaEmbedEndpoint, body)
	if err != nil {
		return nil, err
	}

	var out ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("failed to decode ollama embed response: %w", err)
	}

	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama returned %d embeddings for %d inputs", len(out.Embeddings), len(texts))
	}
	if err := validateDimensions(out.Embeddings, o.dimension); err != nil {
		return nil, err
	}

	return &EmbeddingResult{
		Vectors:  out.Embeddings,
		Provider: o.Provider(),
		Model:    o.model,
		Version:  ollamaEmbeddingVersion,
	}, nil
}

// EmbedQuery produces a dense vector embedding for a single query string.
// Queries are prefixed with "search_query: " per Nomic retrieval guidance.
func (o *OllamaEmbeddingProvider) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(map[string]interface{}{
		"model":  o.model,
		"prompt": ollamaQueryPrefix + query,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama embedding request: %w", err)
	}

	respBody, err := o.doPost(ctx, ollamaEmbeddingsEndpoint, body)
	if err != nil {
		return nil, err
	}

	var out ollamaEmbeddingResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("failed to decode ollama embedding response: %w", err)
	}
	if len(out.Embedding) == 0 {
		return nil, errors.New("ollama returned empty embedding vector")
	}
	if len(out.Embedding) != o.dimension {
		return nil, fmt.Errorf("ollama returned dimension %d, expected %d", len(out.Embedding), o.dimension)
	}
	return out.Embedding, nil
}

// doPost performs a JSON POST to the given Ollama endpoint and returns the response body.
func (o *OllamaEmbeddingProvider) doPost(ctx context.Context, endpoint string, body []byte) ([]byte, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}()

	req.SetRequestURI(o.baseURL + endpoint)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentType("application/json")
	req.SetBody(body)

	if err := o.client.DoTimeout(req, resp, o.timeout); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("ollama request to %s failed: %w", endpoint, err)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, fmt.Errorf("ollama endpoint %s returned status %d: %s", endpoint, resp.StatusCode(), string(resp.Body()))
	}

	return resp.Body(), nil
}

// ollamaEmbeddingResponse models the single-prompt /api/embeddings response.
type ollamaEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// ollamaEmbedResponse models the batch /api/embed response.
type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func validateDimensions(vectors [][]float32, dimension int) error {
	for i, v := range vectors {
		if len(v) != dimension {
			return fmt.Errorf("ollama embedding %d has dimension %d, expected %d", i, len(v), dimension)
		}
	}
	return nil
}
