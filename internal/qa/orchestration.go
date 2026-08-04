package qa

// RetrievalRoutingDecision is the minimal internal M5 orchestration output:
// the only runtime decision currently validated by benchmark evidence is
// whether comparison decomposition applies (ADR-0031, ADR-0032). Retrieval
// mode, TopK, thresholds, and fusion parameters are owned by existing
// configuration and the frozen M4 machinery, not by orchestration.
type RetrievalRoutingDecision struct {
	Decompose bool
}

// DecideRetrievalRouting maps an analyzed query to the M5 routing decision.
// Comparison queries are identified solely by the existing analyzer signal:
// a non-empty SubQueries result means the deterministic comparison
// decomposition (M4) applies. All other queries keep the Balanced path
// without decomposition.
func DecideRetrievalRouting(analyzed *AnalyzedQuery) RetrievalRoutingDecision {
	if analyzed != nil && len(analyzed.SubQueries) > 0 {
		return RetrievalRoutingDecision{Decompose: true}
	}
	return RetrievalRoutingDecision{}
}
