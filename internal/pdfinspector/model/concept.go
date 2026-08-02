package model

// ConceptSource defines the typed provenance of an extracted concept.
type ConceptSource string

const (
	ConceptSourceRuleBased ConceptSource = "rule_based"
	ConceptSourceLLM       ConceptSource = "llm"
	ConceptSourceHybrid    ConceptSource = "hybrid"
)

// Concept represents a minimal abstract topic or thematic metadata record.
type Concept struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Score  float64       `json:"score"`
	Source ConceptSource `json:"source"`
}
