package provider

import (
	"context"
)

// Usage tracks token consumption metrics for embedding generation requests.
type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ProviderCapabilities encapsulates constraints and features supported by an embedding provider.
type ProviderCapabilities struct {
	Dimension      int  `json:"dimension"`
	MaxBatchSize   int  `json:"max_batch_size"`
	MaxInputTokens int  `json:"max_input_tokens"`
	SupportsBatch  bool `json:"supports_batch"`
}

// EmbeddingResult contains the output vectors and metadata returned by an embedding provider.
type EmbeddingResult struct {
	Vectors  [][]float32 `json:"vectors"`
	Usage    Usage       `json:"usage"`
	Provider string      `json:"provider"`
	Model    string      `json:"model"`
	Version  string      `json:"version,omitempty"`
}

// EmbeddingProvider defines the deep module interface seam for LLM embedding generation.
type EmbeddingProvider interface {
	// GenerateEmbeddings produces dense vector embeddings for a slice of input texts.
	GenerateEmbeddings(ctx context.Context, texts []string) (*EmbeddingResult, error)

	// Capabilities returns provider operational bounds (dimension, batch size, token limits).
	Capabilities() ProviderCapabilities

	// Provider returns the canonical provider identifier (e.g. "OpenAI", "mock-provider").
	Provider() string

	// Model returns the embedding model identifier used by this provider.
	Model() string

	// Health checks provider API connectivity and service health.
	Health(ctx context.Context) error
}
