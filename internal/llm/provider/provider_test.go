package provider_test

import (
	"context"
	"testing"

	llmprovider "arca/internal/llm/provider"
	qaprompt "arca/internal/qa/prompt"
)

func TestMockLLMProvider(t *testing.T) {
	ctx := context.Background()
	mock := llmprovider.NewMockLLMProvider("mock-openai", "gpt-4o")

	t.Run("returns correct model capabilities", func(t *testing.T) {
		caps := mock.Capabilities()
		if !caps.SupportsStreaming {
			t.Error("expected MockLLMProvider to support streaming")
		}
		if caps.ContextWindow <= 0 {
			t.Errorf("expected ContextWindow > 0, got %d", caps.ContextWindow)
		}
	})

	t.Run("generates response text incorporating citation markers", func(t *testing.T) {
		prompt := qaprompt.PromptMessage{
			System: "You are an assistant.",
			Messages: []qaprompt.Message{
				{Role: "user", Content: "What is creativity?"},
			},
		}

		res, err := mock.Generate(ctx, prompt)
		if err != nil {
			t.Fatalf("unexpected error during generation: %v", err)
		}

		if res == nil {
			t.Fatal("expected non-nil LLMResponse")
		}
		if res.Content == "" {
			t.Error("expected non-empty response content")
		}
		if res.TokenUsage.TotalTokens <= 0 {
			t.Errorf("expected TotalTokens > 0, got %d", res.TokenUsage.TotalTokens)
		}
	})
}
