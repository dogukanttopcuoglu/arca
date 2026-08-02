package provider

import (
	"context"

	qaprompt "arca/internal/qa/prompt"
)

// LLMUsage tracks prompt, completion, and total token usage metrics.
type LLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelCapabilities encapsulates provider features and context limits.
type ModelCapabilities struct {
	SupportsSystemMessage bool `json:"supports_system_message"`
	SupportsStreaming     bool `json:"supports_streaming"`
	ContextWindow         int  `json:"context_window"`
}

// LLMResponse contains generated text output and execution metadata.
type LLMResponse struct {
	Content    string   `json:"content"`
	Model      string   `json:"model"`
	Provider   string   `json:"provider"`
	TokenUsage LLMUsage `json:"token_usage"`
}

// StreamChunk represents a streamed token fragment from an LLM.
type StreamChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Error   error  `json:"error,omitempty"`
}

// LLMProvider defines the deep module interface seam for LLM text generation and token streaming.
type LLMProvider interface {
	// Generate executes a synchronous completion request for a PromptMessage.
	Generate(ctx context.Context, prompt qaprompt.PromptMessage) (*LLMResponse, error)

	// Stream returns a channel streaming token fragments for a PromptMessage.
	Stream(ctx context.Context, prompt qaprompt.PromptMessage) (<-chan StreamChunk, error)

	// Capabilities returns provider operational bounds and supported features.
	Capabilities() ModelCapabilities
}
