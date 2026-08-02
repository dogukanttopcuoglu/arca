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
	provider provider.EmbeddingProvider
	store    store.VectorStore
}

// NewDenseRetriever constructs a DenseRetriever instance.
func NewDenseRetriever(p provider.EmbeddingProvider, s store.VectorStore) *DenseRetriever {
	return &DenseRetriever{
		provider: p,
		store:    s,
	}
}

// Retrieve embeds query text and performs nearest-neighbor vector search via VectorStore.
func (r *DenseRetriever) Retrieve(ctx context.Context, query seam.RetrievalQuery) ([]seam.SearchResult, error) {
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid retrieval query: %w", err)
	}

	query.Normalize()

	// 1. Generate query text embedding vector
	embResult, err := r.provider.GenerateEmbeddings(ctx, []string{query.QueryText})
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	if len(embResult.Vectors) == 0 {
		return nil, fmt.Errorf("embedding provider returned empty vector slice")
	}

	queryVector := embResult.Vectors[0]

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
	for i, res := range storeResults {
		searchResults[i] = seam.SearchResult{
			ChunkID:  res.Metadata.ChunkID,
			Score:    res.Score,
			Metadata: res.Metadata,
		}
	}

	seam.SortResultsByScore(searchResults)

	return searchResults, nil
}
