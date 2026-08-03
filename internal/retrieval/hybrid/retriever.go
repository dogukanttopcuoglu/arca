package hybrid

import (
	"context"
	"fmt"
	"time"

	"arca/internal/retrieval/seam"
)

// FusionPolicy is the frozen, calibrated retrieval fusion configuration
// produced by offline benchmark calibration (M4) — never raw tuned
// parameters. The orchestrator may select between named policies; numerical
// optimization ends at calibration.
type FusionPolicy struct {
	DenseWeight  float64 // weight of the dense stream in weighted RRF
	SparseWeight float64 // weight of the sparse stream in weighted RRF
	SparseCap    int     // max sparse candidates entering fusion; <=0 = unlimited
	RRFK         float64 // RRF constant (default 60)
}

// DefaultFusionPolicy is the M3-compatible policy (Balanced): equal weights,
// no cap, k=60. It is the backward-compatibility contract — hybrid behavior
// under the default policy must reproduce the frozen M3 baseline exactly.
func DefaultFusionPolicy() FusionPolicy {
	return FusionPolicy{
		DenseWeight:  1.0,
		SparseWeight: 1.0,
		SparseCap:    0,
		RRFK:         60,
	}
}

// HybridRetriever combines Dense vector search and Sparse BM25 search streams using RRF score fusion.
// It is a pure composition layer: the FusionPolicy is the only tuning surface.
type HybridRetriever struct {
	denseRetriever  seam.Retriever
	sparseRetriever seam.Retriever
	policy          FusionPolicy
}

// HybridOption configures a HybridRetriever instance.
type HybridOption func(*HybridRetriever)

// WithFusionPolicy sets the fusion policy (default: DefaultFusionPolicy).
func WithFusionPolicy(p FusionPolicy) HybridOption {
	return func(h *HybridRetriever) {
		h.policy = p
	}
}

// NewHybridRetriever constructs a composite HybridRetriever instance.
func NewHybridRetriever(dense, sparse seam.Retriever, opts ...HybridOption) *HybridRetriever {
	h := &HybridRetriever{
		denseRetriever:  dense,
		sparseRetriever: sparse,
		policy:          DefaultFusionPolicy(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// SetFusionPolicy replaces the active policy (sweep/runtime seam).
func (h *HybridRetriever) SetFusionPolicy(p FusionPolicy) {
	h.policy = p
}

// FusionPolicy returns the active fusion policy.
func (h *HybridRetriever) FusionPolicy() FusionPolicy {
	return h.policy
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
	var weights []float64

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
			weights = append(weights, h.policy.DenseWeight)
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
			// Apply the sparse candidate cap before fusion: only the strongest
			// sparse candidates may enter, so crowding is bounded at the source.
			if h.policy.SparseCap > 0 && len(sparseResults) > h.policy.SparseCap {
				sparseResults = sparseResults[:h.policy.SparseCap]
			}
			streams = append(streams, sparseResults)
			weights = append(weights, h.policy.SparseWeight)
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

	// Fuse streams via weighted Reciprocal Rank Fusion (deterministic: score
	// desc, ChunkID asc on ties via the shared sort).
	fused := ReciprocalRankFusion(streams, h.policy.RRFK, weights...)

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
