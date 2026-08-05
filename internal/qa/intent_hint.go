package qa

import "strings"

// IntentHint is the minimal M6/M7 signal (ADR-0037/0042): it carries only
// Intent, Decompose, and Source. A hint never decides retrieval behavior —
// the Retrieval Orchestrator holds policy. No confidence score is attached
// until a benchmarked classifier justifies probabilistic semantics.
type IntentHint struct {
	Intent    string
	Decompose bool
	Source    string
}

// Intent hint vocabulary. The deterministic comparison and entity
// classifications are benchmark-proven; every other query is "other".
const (
	HintIntentComparison = "comparison"
	HintIntentEntity     = "entity"
	HintIntentOther      = "other"
	HintSourceRuleBased  = "rule_based"
)

// AnalyzeIntentHint produces the IntentHint from the analyzer signal: a
// non-empty SubQueries result means comparison; the analyzer's entity_lookup
// intent means entity (M7, ADR-0042 — the calibration gold set's entity
// queries share that classification). Retrieval-derived signals never enter
// the hint.
func AnalyzeIntentHint(analyzed *AnalyzedQuery) IntentHint {
	if analyzed == nil {
		return IntentHint{Intent: HintIntentOther, Source: HintSourceRuleBased}
	}
	if len(analyzed.SubQueries) > 0 {
		return IntentHint{
			Intent:    HintIntentComparison,
			Decompose: true,
			Source:    HintSourceRuleBased,
		}
	}
	if strings.TrimSpace(analyzed.Intent) == "entity_lookup" {
		return IntentHint{
			Intent: HintIntentEntity,
			Source: HintSourceRuleBased,
		}
	}
	return IntentHint{
		Intent: HintIntentOther,
		Source: HintSourceRuleBased,
	}
}
