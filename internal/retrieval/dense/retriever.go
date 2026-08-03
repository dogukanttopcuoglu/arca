package dense

import (
	"context"
	"fmt"
	"time"

	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/retrieval/seam"
)

// DenseRetriever bridges LLM embedding providers and vector stores to execute dense vector search queries.
type DenseRetriever struct {
	provider     provider.EmbeddingProvider
	store        store.VectorStore
	contentStore store.ContentStore
}

// NewDenseRetriever constructs a DenseRetriever instance.
func NewDenseRetriever(p provider.EmbeddingProvider, s store.VectorStore, c store.ContentStore) *DenseRetriever {
	return &DenseRetriever{
		provider:     p,
		store:        s,
		contentStore: c,
	}
}

// Retrieve embeds query text and performs nearest-neighbor vector search via VectorStore.
func (r *DenseRetriever) Retrieve(ctx context.Context, query seam.RetrievalQuery) ([]seam.SearchResult, error) {
	start := time.Now()

	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid retrieval query: %w", err)
	}

	query.Normalize()

	stats := query.Stats
	if stats != nil {
		stats.TopKRequested = query.TopK
		stats.MinScore = query.MinScore
	}

	// 1. Generate query text embedding vector
	queryVector, err := r.provider.EmbedQuery(ctx, query.QueryText)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	if len(queryVector) == 0 {
		return nil, fmt.Errorf("embedding provider returned empty query vector")
	}

	// 2. Execute nearest neighbor search against VectorStore
	storeResults, err := r.store.SearchVector(ctx, store.VectorSearchQuery{
		Vector:   queryVector,
		TopK:     query.TopK,
		Filter:   query.Filter,
		MinScore: query.MinScore,
	})
	if err != nil {
		return nil, fmt.Errorf("vector store search failed: %w", err)
	}

	// 3. Map VectorSearchResult objects to domain SearchResult objects
	searchResults := make([]seam.SearchResult, len(storeResults))
	chunkIDs := make([]string, 0, len(storeResults))
	for i, res := range storeResults {
		if res.ContentMarkdown == "" {
			chunkIDs = append(chunkIDs, res.Metadata.ChunkID)
		}
		searchResults[i] = seam.SearchResult{
			ChunkID:         res.Metadata.ChunkID,
			Score:           res.Score,
			Metadata:        res.Metadata,
			ContentMarkdown: res.ContentMarkdown,
		}
	}

	// 4. Resolve chunk markdown content from ContentStore only when the vector
	// store did not carry it, so persistent content survives process boundaries
	// while the ContentStore seam remains the in-process fallback.
	if len(chunkIDs) > 0 {
		contents, err := r.contentStore.GetContent(ctx, chunkIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve chunk content: %w", err)
		}
		for i := range searchResults {
			if searchResults[i].ContentMarkdown == "" {
				searchResults[i].ContentMarkdown = contents[searchResults[i].ChunkID]
			}
		}
	}

	seam.SortResultsByScore(searchResults)

	if stats != nil {
		stats.Candidates = len(searchResults)
		stats.TopKReturned = len(searchResults)
		stats.DurationMs = time.Since(start).Milliseconds()
	}

	return searchResults, nil
}
