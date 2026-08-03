package seam

import (
	"context"
)

// Retriever defines the clean domain interface seam for retrieving relevant knowledge chunks for RAG queries.
type Retriever interface {
	// Retrieve searches for relevant KnowledgeChunks matching query text and filters.
	Retrieve(ctx context.Context, query RetrievalQuery) ([]SearchResult, error)
}

// RetrievalStats captures per-retrieval diagnostics for benchmarking and
// observability. It is populated only when the caller attaches a pointer via
// RetrievalQuery.Stats; nil leaves retrieval behavior unchanged.
type RetrievalStats struct {
	DurationMs       int64   `json:"duration_ms"`
	Candidates       int     `json:"candidates"`         // results received from the store before final truncation
	TopKRequested    int     `json:"top_k_requested"`    // TopK after normalization
	TopKReturned     int     `json:"top_k_returned"`     // final result count
	MinScore         float32 `json:"min_score"`          // threshold applied
	DenseCandidates  int     `json:"dense_candidates,omitempty"`   // hybrid only
	SparseCandidates int     `json:"sparse_candidates,omitempty"`  // hybrid only
	FusedCandidates  int     `json:"fused_candidates,omitempty"`   // hybrid only
}
