package store

import (
	"context"
	"fmt"
	"sync"
)

// ChunkContent associates a chunk identifier with its raw markdown content.
type ChunkContent struct {
	ChunkID         string `json:"chunk_id"`
	ContentMarkdown string `json:"content_markdown"`
}

// ContentStore defines the persistence abstraction for chunk markdown content,
// kept separate from vector points so the vector store payload stays lean and
// retrieval metadata does not bloat with full document text.
type ContentStore interface {
	// PutContent stores or replaces markdown content for the given chunks.
	PutContent(ctx context.Context, chunks []ChunkContent) error

	// GetContent returns markdown content keyed by ChunkID for the requested chunk IDs.
	GetContent(ctx context.Context, chunkIDs []string) (map[string]string, error)

	// DeleteContent removes stored content for the given chunk IDs.
	DeleteContent(ctx context.Context, chunkIDs []string) error

	// Health checks store connectivity and availability.
	Health(ctx context.Context) error
}

// InMemoryContentStore is a thread-safe in-memory adapter for ContentStore.
type InMemoryContentStore struct {
	mu       sync.RWMutex
	contents map[string]string
}

// NewInMemoryContentStore constructs an InMemoryContentStore instance.
func NewInMemoryContentStore() *InMemoryContentStore {
	return &InMemoryContentStore{
		contents: make(map[string]string),
	}
}

// Health checks in-memory content store status.
func (s *InMemoryContentStore) Health(ctx context.Context) error {
	return nil
}

// PutContent stores or replaces markdown content for the given chunks.
func (s *InMemoryContentStore) PutContent(ctx context.Context, chunks []ChunkContent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ch := range chunks {
		if ch.ChunkID == "" {
			return fmt.Errorf("chunk content chunk ID cannot be empty")
		}
		s.contents[ch.ChunkID] = ch.ContentMarkdown
	}
	return nil
}

// GetContent returns markdown content keyed by ChunkID for the requested chunk IDs.
func (s *InMemoryContentStore) GetContent(ctx context.Context, chunkIDs []string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]string, len(chunkIDs))
	for _, id := range chunkIDs {
		if content, ok := s.contents[id]; ok {
			result[id] = content
		}
	}
	return result, nil
}

// DeleteContent removes stored content for the given chunk IDs.
func (s *InMemoryContentStore) DeleteContent(ctx context.Context, chunkIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range chunkIDs {
		delete(s.contents, id)
	}
	return nil
}
