package hybrid

import (
	"context"
	"fmt"

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
func (h *HybridRetriever) Retrieve(ctx context.Context, query seam.RetrievalQuery) ([]seam.SearchResult, error) {
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid retrieval query: %w", err)
	}

	query.Normalize()

	var streams [][]seam.SearchResult

	// Execute Dense search
	if h.denseRetriever != nil {
		denseQuery := query
		denseQuery.Mode = seam.RetrievalDense
		denseResults, err := h.denseRetriever.Retrieve(ctx, denseQuery)
		if err == nil && len(denseResults) > 0 {
			streams = append(streams, denseResults)
		}
	}

	// Execute Sparse search
	if h.sparseRetriever != nil {
		sparseQuery := query
		sparseQuery.Mode = seam.RetrievalSparse
		sparseResults, err := h.sparseRetriever.Retrieve(ctx, sparseQuery)
		if err == nil && len(sparseResults) > 0 {
			streams = append(streams, sparseResults)
		}
	}

	if len(streams) == 0 {
		return []seam.SearchResult{}, nil
	}

	// Fuse streams via Reciprocal Rank Fusion
	fused := ReciprocalRankFusion(streams, h.rrfKConst)

	if len(fused) > query.TopK {
		fused = fused[:query.TopK]
	}

	return fused, nil
}
