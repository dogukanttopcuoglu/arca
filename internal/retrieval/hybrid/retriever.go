package hybrid

import (
	"context"
	"fmt"
	"time"

	"arca/internal/retrieval/seam"
)

// HybridRetriever combines Dense vector search and Sparse BM25 search streams using RRF score fusion.
type HybridRetriever struct {
	denseRetriever  seam.Retriever
	sparseRetriever seam.Retriever
	rrfKConst       float64
}

// NewHybridRetriever constructs a composite HybridRetriever instance.
func NewHybridRetriever(dense, sparse seam.Retriever) *HybridRetriever {
	return &HybridRetriever{
		denseRetriever:  dense,
		sparseRetriever: sparse,
		rrfKConst:       60.0,
	}
}

// SetRRFConstant updates the RRF formula constant k (default 60.0).
func (h *HybridRetriever) SetRRFConstant(k float64) {
	if k > 0 {
		h.rrfKConst = k
	}
}

// Retrieve executes parallel/composite retrieval across sub-retrievers and fuses results via RRF.
// The hybrid retriever is a pure composition layer: no scoring logic beyond RRF, no tokenizer,
// no query preprocessing — dense and sparse streams are orchestrated as-is.
func (h *HybridRetriever) Retrieve(ctx context.Context, query seam.RetrievalQuery) ([]seam.SearchResult, error) {
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

	var streams [][]seam.SearchResult

	// Execute Dense search. Sub-retrievers receive detached stats: they must
	// never write into the shared aggregate (the hybrid counts stream lengths
	// itself).
	if h.denseRetriever != nil {
		denseQuery := query
		denseQuery.Mode = seam.RetrievalDense
		denseQuery.Stats = nil
		denseResults, err := h.denseRetriever.Retrieve(ctx, denseQuery)
		if err == nil && len(denseResults) > 0 {
			streams = append(streams, denseResults)
		}
		if stats != nil {
			stats.DenseCandidates = len(denseResults)
		}
	}

	// Execute Sparse search
	if h.sparseRetriever != nil {
		sparseQuery := query
		sparseQuery.Mode = seam.RetrievalSparse
		sparseQuery.Stats = nil
		sparseResults, err := h.sparseRetriever.Retrieve(ctx, sparseQuery)
		if err == nil && len(sparseResults) > 0 {
			streams = append(streams, sparseResults)
		}
		if stats != nil {
			stats.SparseCandidates = len(sparseResults)
		}
	}

	if len(streams) == 0 {
		if stats != nil {
			stats.DurationMs = time.Since(start).Milliseconds()
		}
		return []seam.SearchResult{}, nil
	}

	// Fuse streams via Reciprocal Rank Fusion (deterministic: score desc,
	// ChunkID asc on ties via the shared sort).
	fused := ReciprocalRankFusion(streams, h.rrfKConst)

	if len(fused) > query.TopK {
		fused = fused[:query.TopK]
	}

	if stats != nil {
		stats.FusedCandidates = len(fused)
		stats.TopKReturned = len(fused)
		stats.DurationMs = time.Since(start).Milliseconds()
	}

	return fused, nil
}
