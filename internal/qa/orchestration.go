package qa

// RetrievalDecision is the output of the Retrieval Orchestrator (ADR-0037,
// extended by ADR-0042 after the M7 benchmark acceptance): comparison
// decomposition, the comparison evidence-budget TopK override, and the
// benchmark-gated graph gate for entity queries. A zero TopKOverride means
// "use the caller's TopK"; UseGraph is honored only when a graph retriever is
// injected into the engine.
type RetrievalDecision struct {
	Decompose    bool
	TopKOverride int
	UseGraph     bool
}

// RetrievalRuntimeConfig carries the benchmark-calibrated orchestration
// parameters. ComparisonTopK is the evidence budget for comparison queries
// (0 = unset; the caller's TopK applies). GraphWeight is the frozen M7 graph
// fusion weight (1.0, ADR-0041 calibration); 0 keeps the graph gate closed.
type RetrievalRuntimeConfig struct {
	ComparisonTopK int
	GraphWeight    float64
}

// DecideRetrievalRouting is the Retrieval Orchestrator: a pure decision
// function translating an IntentHint and runtime config into a
// RetrievalDecision. Comparison hints get decomposition and the calibrated
// evidence budget; entity hints open the graph gate when a positive graph
// weight is configured (ADR-0042). All other hints keep the Balanced path.
func DecideRetrievalRouting(hint IntentHint, cfg RetrievalRuntimeConfig) RetrievalDecision {
	switch hint.Intent {
	case HintIntentComparison:
		decision := RetrievalDecision{Decompose: true}
		if cfg.ComparisonTopK > 0 {
			decision.TopKOverride = cfg.ComparisonTopK
		}
		return decision
	case HintIntentEntity:
		if cfg.GraphWeight > 0 {
			return RetrievalDecision{UseGraph: true}
		}
	}
	return RetrievalDecision{}
}
