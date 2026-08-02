package prompt

import (
	"context"
	"fmt"
	"strings"

	qacontext "arca/internal/qa/context"
)

// Message represents a single chat turn (role and content).
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerationOptions configures LLM temperature and max tokens.
type GenerationOptions struct {
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

// PromptMessage represents a vendor-agnostic prompt payload.
type PromptMessage struct {
	System   string            `json:"system"`
	Messages []Message         `json:"messages"`
	Options  GenerationOptions `json:"options"`
}

// PromptBuilder defines the seam for building prompt messages from query and context window.
type PromptBuilder interface {
	Build(ctx context.Context, query string, win *qacontext.ContextWindow) (PromptMessage, error)
}

// RAGPromptBuilder implements PromptBuilder constructing system instructions and RAG citation constraints.
type RAGPromptBuilder struct{}

// NewRAGPromptBuilder constructs a RAGPromptBuilder instance.
func NewRAGPromptBuilder() *RAGPromptBuilder {
	return &RAGPromptBuilder{}
}

const defaultSystemInstruction = `You are ARC Knowledge OS, an accurate, evidence-backed Document Intelligence Assistant.
Answer the user query using ONLY the provided Source Context blocks.
RULES:
1. Always cite your claims using inline reference markers like [Ref 1], [Ref 2] corresponding to the source blocks.
2. Never invent or hallucinate facts outside the provided sources.
3. If the provided context does not contain enough information to answer, state clearly that the sources do not specify.`

// Build formats query and context window into a PromptMessage.
func (b *RAGPromptBuilder) Build(ctx context.Context, query string, win *qacontext.ContextWindow) (PromptMessage, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return PromptMessage{}, fmt.Errorf("query text cannot be empty")
	}

	var userMsg strings.Builder
	userMsg.WriteString("CONTEXT:\n")
	if win != nil && win.Content != "" {
		userMsg.WriteString(win.Content)
	} else {
		userMsg.WriteString("No source context available.\n")
	}
	userMsg.WriteString("\nUSER QUESTION:\n")
	userMsg.WriteString(trimmedQuery)

	return PromptMessage{
		System: defaultSystemInstruction,
		Messages: []Message{
			{Role: "user", Content: userMsg.String()},
		},
		Options: GenerationOptions{
			Temperature: 0.2,
			MaxTokens:   1500,
		},
	}, nil
}
