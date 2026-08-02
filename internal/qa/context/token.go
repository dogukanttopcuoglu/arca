package context

import (
	"strings"
)

// TokenCounter defines the seam for estimating or calculating token counts for prompt text.
type TokenCounter interface {
	// Count returns the number of tokens in the given text.
	Count(text string) int
}

// SimpleTokenCounter is a lightweight rule-based tokenizer adapter (approx ~4 chars per token).
type SimpleTokenCounter struct{}

// NewSimpleTokenCounter constructs a SimpleTokenCounter instance.
func NewSimpleTokenCounter() *SimpleTokenCounter {
	return &SimpleTokenCounter{}
}

// Count estimates token count based on whitespace word splits and character count.
func (c *SimpleTokenCounter) Count(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	words := len(strings.Fields(trimmed))
	charTokens := len(trimmed) / 4
	if charTokens > words {
		return charTokens
	}
	if words == 0 {
		return 1
	}
	return words
}
