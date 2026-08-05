package qa

// RetrievalDecision is the output of the Retrieval Orchestrator (ADR-0037).
// It exposes only benchmark-validated decision fields: whether comparison
// decomposition applies, and an optional evidence-budget TopK override.
// Additional fields (e.g. a fusion Policy) require benchmark evidence before
// they are added. A zero TopKOverride means "use the caller's TopK".
type RetrievalDecision struct {
	Decompose    bool
	TopKOverride int
}

// RetrievalRuntimeConfig carries the benchmark-calibrated orchestration
// parameters. ComparisonTopK is the evidence budget for comparison queries
// (0 = unset; the caller's TopK applies). The concrete value freezes only
// after benchmark calibration on Gold Set v2 (ADR-0037).
type RetrievalRuntimeConfig struct {
	ComparisonTopK int
}

// DecideRetrievalRouting is the Retrieval Orchestrator: a pure decision
// function translating an IntentHint and runtime config into a
// RetrievalDecision. Comparison hints may additionally receive the calibrated
// evidence budget; all other hints keep the Balanced path without
// decomposition or TopK override.
func DecideRetrievalRouting(hint IntentHint, cfg RetrievalRuntimeConfig) RetrievalDecision {
	if !hint.Decompose {
		return RetrievalDecision{}
	}
	decision := RetrievalDecision{Decompose: true}
	if cfg.ComparisonTopK > 0 {
		decision.TopKOverride = cfg.ComparisonTopK
	}
	return decision
}
