package graphfusion

import (
	"context"
	"fmt"

	"arca/internal/retrieval/hybrid"
	retrievalseam "arca/internal/retrieval/seam"
)

// rrfK is the frozen RRF constant (60, the M4 parameter). It is a retriever
// constant, deliberately NOT part of GraphFusionConfig: the ADR-0041 sweep
// covers only the weights; RRFK is out of sweep scope.
const rrfK = 60

// DefaultDenseWeight is the frozen dense-stream weight (1.0, ADR-0041): the
// calibration sweep varied only the graph weight.
const DefaultDenseWeight = 1.0

// GraphFusionConfig is the independent weight surface for the graph fusion
// path (ADR-0041). It mirrors the shape of the frozen M4 FusionPolicy but is
// deliberately separate: runtime graph weighting is calibrated by the
// ADR-0040 benchmark sweep, not by the M4 fusion machinery.
type GraphFusionConfig struct {
	DenseWeight float64
	GraphWeight float64
}

// GraphFusionRetriever fuses the dense and graph retrieval streams with
// weighted RRF (ADR-0041). It is an additive retrieval source: with
// GraphWeight 0 it returns the dense stream unchanged, so production
// behavior is byte-identical until calibration freezes a positive weight.
type GraphFusionRetriever struct {
	dense  retrievalseam.Retriever
	graph  retrievalseam.Retriever
	config GraphFusionConfig
}

// NewGraphFusionRetriever constructs the fusion retriever. A zero config
// means dense-only (GraphWeight 0).
func NewGraphFusionRetriever(dense, graph retrievalseam.Retriever, config GraphFusionConfig) *GraphFusionRetriever {
	return &GraphFusionRetriever{
		dense:  dense,
		graph:  graph,
		config: config,
	}
}

// SetConfig replaces the active fusion config (eval sweep seam, mirroring
// HybridRetriever.SetFusionPolicy).
func (f *GraphFusionRetriever) SetConfig(config GraphFusionConfig) {
	f.config = config
}

// Retrieve executes both streams and fuses them with weighted RRF. With
// GraphWeight 0, or when the graph stream is empty, the dense stream is
// returned unchanged (deterministic degradation). Graph-only measurement is
// not a mode of this retriever — the eval surface uses the graph retriever
// directly for --graph-only runs (ticket 05); RRF maps non-positive weights
// to 1.0, so DenseWeight 0 here would mean balanced, not graph-only.
func (f *GraphFusionRetriever) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}
	query.Normalize()

	denseResults, err := f.dense.Retrieve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("dense retrieval failed: %w", err)
	}

	if f.config.GraphWeight <= 0 || f.graph == nil {
		return truncate(denseResults, query.TopK), nil
	}

	graphResults, err := f.graph.Retrieve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("graph retrieval failed: %w", err)
	}
	if len(graphResults) == 0 {
		return truncate(denseResults, query.TopK), nil
	}

	fused := hybrid.ReciprocalRankFusion(
		[][]retrievalseam.SearchResult{denseResults, graphResults},
		rrfK,
		f.config.DenseWeight,
		f.config.GraphWeight,
	)

	return truncate(fused, query.TopK), nil
}

func truncate(results []retrievalseam.SearchResult, topK int) []retrievalseam.SearchResult {
	if topK <= 0 {
		topK = 10
	}
	if len(results) > topK {
		return results[:topK]
	}
	return results
}
