package chunking

import "strings"

// ChunkSizer abstracts the strategy for calculating chunk size (e.g. token count).
type ChunkSizer interface {
	Size(text string) int
}

// HeuristicSizer provides a fast, tokenizer-independent heuristic estimate.
type HeuristicSizer struct{}

// NewHeuristicSizer creates a default HeuristicSizer.
func NewHeuristicSizer() *HeuristicSizer {
	return &HeuristicSizer{}
}

// Size estimates tokens using character length / 4 with word count bounds.
func (s *HeuristicSizer) Size(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	words := len(strings.Fields(trimmed))
	chars := len([]rune(trimmed))

	charEstimate := (chars + 3) / 4
	wordEstimate := int(float64(words) * 1.3)

	if charEstimate > wordEstimate {
		return charEstimate
	}
	return wordEstimate
}
