// Package sparse implements the sparse lexical retriever: it encodes the
// query through a SparseEncoder and searches the vector store's named sparse
// field. It mirrors the dense retriever exactly — only the encoder differs.
package sparse

import (
	"context"
	"fmt"
	"time"

	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
	"arca/internal/retrieval/seam"
)

// SparseRetriever searches by sparse vector similarity. Symmetric to the
// dense retriever: same query handling, TopK/MinScore semantics, content
// resolution fallback, and statistics collection.
type SparseRetriever struct {
	encoder      sparse.SparseEncoder
	store        store.VectorStore
	contentStore store.ContentStore
}

// NewSparseRetriever constructs a SparseRetriever. The encoder must be bound
// to the same corpus statistics used at indexing time so query term ids
// align with stored sparse vectors.
func NewSparseRetriever(enc sparse.SparseEncoder, s store.VectorStore, c store.ContentStore) *SparseRetriever {
	return &SparseRetriever{
		encoder:      enc,
		store:        s,
		contentStore: c,
	}
}

// Retrieve encodes the query text, executes sparse nearest-neighbor search,
// and maps results to domain SearchResults with content resolution.
func (r *SparseRetriever) Retrieve(ctx context.Context, query seam.RetrievalQuery) ([]seam.SearchResult, error) {
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

	// 1. Encode query text into a sparse vector.
	queryVector, err := r.encoder.Encode(ctx, query.QueryText)
	if err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}
	if len(queryVector.Indices) == 0 {
		if stats != nil {
			stats.DurationMs = time.Since(start).Milliseconds()
		}
		return []seam.SearchResult{}, nil
	}

	// 2. Execute sparse nearest-neighbor search against the vector store.
	storeResults, err := r.store.SearchVector(ctx, store.VectorSearchQuery{
		Sparse:   &queryVector,
		TopK:     query.TopK,
		Filter:   query.Filter,
		MinScore: query.MinScore,
	})
	if err != nil {
		return nil, fmt.Errorf("vector store sparse search failed: %w", err)
	}

	// 3. Map VectorSearchResult objects to domain SearchResult objects.
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

	// 4. Resolve content from ContentStore only when the vector store did not
	// carry it — same fallback path as the dense retriever.
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

	// 5. Deterministic ordering: score desc, ChunkID asc on ties.
	seam.SortResultsByScore(searchResults)

	if stats != nil {
		stats.Candidates = len(searchResults)
		stats.TopKReturned = len(searchResults)
		stats.DurationMs = time.Since(start).Milliseconds()
	}

	return searchResults, nil
}
