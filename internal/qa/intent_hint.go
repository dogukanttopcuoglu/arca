package qa

// IntentHint is the minimal M6 signal (ADR-0037): it carries only Intent,
// Decompose, and Source. A hint never decides retrieval behavior — the
// Retrieval Orchestrator holds policy. No confidence score is attached until
// a benchmarked classifier justifies probabilistic semantics.
type IntentHint struct {
	Intent    string
	Decompose bool
	Source    string
}

// Intent hint vocabulary. Only the deterministic comparison classification is
// benchmark-proven today; every other query is "other".
const (
	HintIntentComparison = "comparison"
	HintIntentOther      = "other"
	HintSourceRuleBased  = "rule_based"
)

// AnalyzeIntentHint produces the IntentHint from the existing analyzer
// signal: a non-empty SubQueries result means comparison. Retrieval-derived
// signals never enter the hint.
func AnalyzeIntentHint(analyzed *AnalyzedQuery) IntentHint {
	if analyzed != nil && len(analyzed.SubQueries) > 0 {
		return IntentHint{
			Intent:    HintIntentComparison,
			Decompose: true,
			Source:    HintSourceRuleBased,
		}
	}
	return IntentHint{
		Intent: HintIntentOther,
		Source: HintSourceRuleBased,
	}
}
