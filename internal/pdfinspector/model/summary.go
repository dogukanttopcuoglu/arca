package model

// SummarySource defines the typed provenance of an extracted summary.
type SummarySource string

const (
	SummarySourceRuleBased SummarySource = "rule_based"
	SummarySourceLLM       SummarySource = "llm"
	SummarySourceHybrid    SummarySource = "hybrid"
)

// Summary represents a minimal domain record encapsulating summary text and provenance.
type Summary struct {
	Text   string        `json:"text"`
	Source SummarySource `json:"source"`
}
