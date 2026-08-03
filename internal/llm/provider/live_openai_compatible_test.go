package provider_test

import (
	"context"
	"os"
	"testing"
	"time"

	llmprovider "arca/internal/llm/provider"
	qaprompt "arca/internal/qa/prompt"
)

// TestLiveOpenAICompatibleProvider runs against the real OpenAI-compatible
// gateway when LLM_API_KEY is set (LLM_BASE_URL optional, defaults to the
// ADR-0026 deployment target). It is skipped otherwise. The test verifies the
// adapter boundary only — connectivity, authentication, request/response
// mapping, and usage parsing — and never touches Firecrawl or Qdrant.
func TestLiveOpenAICompatibleProvider(t *testing.T) {
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		t.Skip("LLM_API_KEY not set; skipping live OpenAI-compatible adapter test")
	}

	baseURL := os.Getenv("LLM_BASE_URL")
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		// Test-harness default; production configuration always supplies LLM_MODEL.
		model = "gpt-4o-mini"
	}

	p := llmprovider.NewOpenAICompatibleProvider(baseURL, apiKey, model, "live-test")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := p.Generate(ctx, qaprompt.PromptMessage{
		System: "You are a terse test assistant. Answer in one sentence.",
		Messages: []qaprompt.Message{
			{Role: "user", Content: "Say connectivity check."},
		},
		Options: qaprompt.GenerationOptions{Temperature: 0, MaxTokens: 64},
	})
	if err != nil {
		t.Fatalf("live generation against gateway failed: %v", err)
	}

	if resp.Content == "" {
		t.Fatal("expected non-empty completion content")
	}
	if resp.Provider != "live-test" {
		t.Errorf("expected provider label from configuration, got %q", resp.Provider)
	}
	if resp.Model == "" {
		t.Error("expected a non-empty model on the response")
	}
	if resp.TokenUsage.TotalTokens <= 0 {
		t.Errorf("expected positive total token usage, got %d", resp.TokenUsage.TotalTokens)
	}
}
