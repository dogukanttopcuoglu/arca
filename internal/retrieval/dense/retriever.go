package dense

import (
	"context"
	"fmt"

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
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid retrieval query: %w", err)
	}

	query.Normalize()

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
	chunkIDs := make([]string, len(storeResults))
	for i, res := range storeResults {
		chunkIDs[i] = res.Metadata.ChunkID
		searchResults[i] = seam.SearchResult{
			ChunkID:  res.Metadata.ChunkID,
			Score:    res.Score,
			Metadata: res.Metadata,
		}
	}

	// 4. Resolve chunk markdown content from ContentStore so QA has the actual text.
	if len(chunkIDs) > 0 {
		contents, err := r.contentStore.GetContent(ctx, chunkIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve chunk content: %w", err)
		}
		for i := range searchResults {
			searchResults[i].ContentMarkdown = contents[searchResults[i].ChunkID]
		}
	}

	seam.SortResultsByScore(searchResults)

	return searchResults, nil
}
