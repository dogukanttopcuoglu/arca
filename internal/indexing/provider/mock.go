package provider

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sync"
)

// ErrProviderUnhealthy is returned when Health check fails.
var ErrProviderUnhealthy = errors.New("embedding provider service is unhealthy")

// MockEmbeddingProvider is a thread-safe mock implementation of EmbeddingProvider for offline testing.
type MockEmbeddingProvider struct {
	mu        sync.RWMutex
	provider  string
	model     string
	dimension int
	healthy   bool
}

// NewMockEmbeddingProvider constructs a MockEmbeddingProvider instance.
func NewMockEmbeddingProvider(providerName, modelName string, dimension int) *MockEmbeddingProvider {
	if dimension <= 0 {
		dimension = 1536
	}
	if providerName == "" {
		providerName = "mock-provider"
	}
	if modelName == "" {
		modelName = "mock-model-v1"
	}
	return &MockEmbeddingProvider{
		provider:  providerName,
		model:     modelName,
		dimension: dimension,
		healthy:   true,
	}
}

// SetHealthy toggles the simulated health state of the mock provider.
func (m *MockEmbeddingProvider) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

// Provider returns the canonical provider identifier.
func (m *MockEmbeddingProvider) Provider() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

// Model returns the embedding model identifier.
func (m *MockEmbeddingProvider) Model() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.model
}

// Capabilities returns mock provider operational bounds.
func (m *MockEmbeddingProvider) Capabilities() ProviderCapabilities {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ProviderCapabilities{
		Dimension:      m.dimension,
		MaxBatchSize:   100,
		MaxInputTokens: 8192,
		SupportsBatch:  true,
	}
}

// Health verifies mock provider service availability.
func (m *MockEmbeddingProvider) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.healthy {
		return ErrProviderUnhealthy
	}
	return nil
}

// GenerateEmbeddings produces deterministic normalized pseudo-vectors for input text strings.
func (m *MockEmbeddingProvider) GenerateEmbeddings(ctx context.Context, texts []string) (*EmbeddingResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.healthy {
		return nil, ErrProviderUnhealthy
	}

	vectors := make([][]float32, len(texts))
	totalTokens := 0

	for i, text := range texts {
		vectors[i] = m.generateDeterministicVector(text)
		totalTokens += len(text) / 4 // Simple token estimation
		if totalTokens == 0 {
			totalTokens = 1
		}
	}

	return &EmbeddingResult{
		Vectors:  vectors,
		Usage:    Usage{PromptTokens: totalTokens, TotalTokens: totalTokens},
		Provider: m.provider,
		Model:    m.model,
		Version:  "1.0.0",
	}, nil
}

func (m *MockEmbeddingProvider) generateDeterministicVector(text string) []float32 {
	vec := make([]float32, m.dimension)
	hash := sha256.Sum256([]byte(text))

	var sumSq float64
	for i := 0; i < m.dimension; i++ {
		// Derive float values deterministically from hash and index
		idx := (i * 4) % (len(hash) - 4)
		seed := binary.BigEndian.Uint32(hash[idx : idx+4])
		val := math.Sin(float64(seed + uint32(i)))
		vec[i] = float32(val)
		sumSq += val * val
	}

	// L2 Normalize vector so norm = 1.0
	norm := math.Sqrt(sumSq)
	if norm > 0 {
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}

	return vec
}
