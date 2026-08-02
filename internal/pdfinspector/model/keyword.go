package model

// KeywordSource defines the typed provenance of an extracted keyword.
type KeywordSource string

const (
	KeywordSourceRuleBased KeywordSource = "rule_based"
	KeywordSourceLLM       KeywordSource = "llm"
	KeywordSourceHybrid    KeywordSource = "hybrid"
)

// Keyword represents structured semantic keyword metadata.
type Keyword struct {
	Value    string        `json:"value"`
	Score    float64       `json:"score"`
	Source   KeywordSource `json:"source"`
	ChunkIDs []string      `json:"chunk_ids,omitempty"`
}
