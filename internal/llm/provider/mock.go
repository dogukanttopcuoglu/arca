package provider

import (
	"context"
	"strings"

	qaprompt "arca/internal/qa/prompt"
)

// MockLLMProvider is an in-memory mock implementation of LLMProvider for offline testing.
type MockLLMProvider struct {
	provider string
	model    string
}

// NewMockLLMProvider constructs a MockLLMProvider instance.
func NewMockLLMProvider(providerName, modelName string) *MockLLMProvider {
	if providerName == "" {
		providerName = "mock-llm-provider"
	}
	if modelName == "" {
		modelName = "mock-model-v1"
	}
	return &MockLLMProvider{
		provider: providerName,
		model:    modelName,
	}
}

// Capabilities returns mock LLM operational bounds.
func (m *MockLLMProvider) Capabilities() ModelCapabilities {
	return ModelCapabilities{
		SupportsSystemMessage: true,
		SupportsStreaming:     true,
		ContextWindow:         128000,
	}
}

// Generate produces a deterministic completion text referencing [Ref 1] if context is present.
func (m *MockLLMProvider) Generate(ctx context.Context, prompt qaprompt.PromptMessage) (*LLMResponse, error) {
	userContent := ""
	if len(prompt.Messages) > 0 {
		userContent = prompt.Messages[0].Content
	}

	content := "Based on the provided information, creativity is a discipline and a lifestyle [Ref 1]."
	if !strings.Contains(userContent, "[Ref 1]") {
		content = "Based on general knowledge, creativity requires practice."
	}

	promptTokens := len(userContent) / 4
	completionTokens := len(content) / 4

	return &LLMResponse{
		Content:  content,
		Model:    m.model,
		Provider: m.provider,
		TokenUsage: LLMUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}, nil
}

// Stream returns a channel producing token chunks for the completion response.
func (m *MockLLMProvider) Stream(ctx context.Context, prompt qaprompt.PromptMessage) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 10)

	go func() {
		defer close(ch)
		res, err := m.Generate(ctx, prompt)
		if err != nil {
			ch <- StreamChunk{Error: err}
			return
		}

		words := strings.Fields(res.Content)
		for i, word := range words {
			text := word
			if i < len(words)-1 {
				text += " "
			}
			ch <- StreamChunk{Content: text}
		}
		ch <- StreamChunk{Done: true}
	}()

	return ch, nil
}
