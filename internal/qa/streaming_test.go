package qa_test

import (
	"context"
	"testing"

	llmprovider "arca/internal/llm/provider"
	"arca/internal/qa"
	retrievalseam "arca/internal/retrieval/seam"
)

func TestStreamingAnswerEngine(t *testing.T) {
	ctx := context.Background()

	mockLLM := llmprovider.NewMockLLMProvider("mock-provider", "mock-model")
	engine := qa.NewStreamingAnswerEngine(nil, nil, nil, nil, mockLLM)

	t.Run("streams tokens and produces final verification chunk", func(t *testing.T) {
		ch, err := engine.AnswerStream(ctx, retrievalseam.RetrievalQuery{
			QueryText: "What is creativity?",
		})
		if err != nil {
			t.Fatalf("unexpected error starting stream: %v", err)
		}

		var chunks []qa.AnswerStreamChunk
		for chunk := range ch {
			chunks = append(chunks, chunk)
		}

		if len(chunks) == 0 {
			t.Fatal("expected non-empty stream chunks")
		}

		// Last chunk should be verification or done event
		lastChunk := chunks[len(chunks)-1]
		if lastChunk.Type != qa.StreamChunkVerification && lastChunk.Type != qa.StreamChunkDone {
			t.Errorf("expected final chunk type verification or done, got %s", lastChunk.Type)
		}
	})
}
