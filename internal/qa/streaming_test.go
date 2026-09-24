package qa_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	llmprovider "arca/internal/llm/provider"
	"arca/internal/qa"
	qaverification "arca/internal/qa/verification"
	retrievalseam "arca/internal/retrieval/seam"
)

func TestAnswerEngine_AnswerStream(t *testing.T) {
	ctx := context.Background()

	t.Run("empty query returns an error immediately", func(t *testing.T) {
		engine := qa.NewAnswerEngine(nil, nil, nil, nil, nil, nil, nil)

		ch, err := engine.AnswerStream(ctx, retrievalseam.RetrievalQuery{})
		if err == nil {
			t.Fatal("expected error for empty query, got nil")
		}
		if ch != nil {
			t.Error("expected nil stream channel for empty query")
		}
	})

	t.Run("abstains with no_evidence and no tokens when retrieval has no sources", func(t *testing.T) {
		engine := qa.NewAnswerEngine(nil, nil, nil, nil, nil, nil, nil)

		ch, err := engine.AnswerStream(ctx, retrievalseam.RetrievalQuery{QueryText: "What is creativity?"})
		if err != nil {
			t.Fatalf("unexpected error starting stream: %v", err)
		}

		var chunks []qa.AnswerStreamChunk
		for chunk := range ch {
			chunks = append(chunks, chunk)
		}

		if len(chunks) != 2 {
			t.Fatalf("expected verification then done, got %d chunks", len(chunks))
		}
		if chunks[0].Type != qa.StreamChunkVerification {
			t.Errorf("expected first chunk verification, got %s", chunks[0].Type)
		}
		if chunks[0].Verified == nil || chunks[0].Verified.Status != qaverification.StatusNoEvidence {
			t.Errorf("expected no_evidence verification, got %+v", chunks[0].Verified)
		}
		if chunks[0].Verified.Text != "The retrieved sources do not cover this query, so no grounded answer can be provided." {
			t.Errorf("expected abstention text, got %q", chunks[0].Verified.Text)
		}
		if chunks[1].Type != qa.StreamChunkDone {
			t.Errorf("expected final chunk done, got %s", chunks[1].Type)
		}
	})

	t.Run("streams tokens and emits verified final chunk", func(t *testing.T) {
		retriever := &scriptedRetriever{
			byQuery: map[string][]retrievalseam.SearchResult{
				"What is creativity?": {sr("chk-1")},
			},
		}
		mockLLM := llmprovider.NewMockLLMProvider("mock-provider", "mock-model")
		engine := newTestEngine(retriever, mockLLM, 4000)

		ch, err := engine.AnswerStream(ctx, retrievalseam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error starting stream: %v", err)
		}

		var chunks []qa.AnswerStreamChunk
		for chunk := range ch {
			chunks = append(chunks, chunk)
		}

		var tokens []string
		var verified *qaverification.VerifiedAnswer
		for _, chunk := range chunks {
			switch chunk.Type {
			case qa.StreamChunkToken:
				tokens = append(tokens, chunk.Content)
			case qa.StreamChunkVerification:
				verified = chunk.Verified
			case qa.StreamChunkError:
				t.Errorf("unexpected error chunk: %s", chunk.Error)
			}
		}

		if len(tokens) == 0 {
			t.Fatal("expected token chunks from streaming provider")
		}
		want := "Based on the provided information, creativity is a discipline and a lifestyle [Ref 1]."
		if full := strings.Join(tokens, ""); full != want {
			t.Errorf("expected assembled stream %q, got %q", want, full)
		}
		if verified == nil {
			t.Fatal("expected final verification chunk")
		}
		if verified.Text != want {
			t.Errorf("expected verified text %q, got %q", want, verified.Text)
		}
		if verified.Status != qaverification.StatusVerified {
			t.Errorf("expected verified status %q, got %q", qaverification.StatusVerified, verified.Status)
		}
		if last := chunks[len(chunks)-1]; last.Type != qa.StreamChunkDone {
			t.Errorf("expected final chunk done, got %s", last.Type)
		}
	})

	t.Run("emits an error chunk when retrieval fails", func(t *testing.T) {
		retriever := &errRetriever{err: errors.New("boom")}
		engine := newTestEngine(retriever, llmprovider.NewMockLLMProvider("mock-provider", "mock-model"), 4000)

		ch, err := engine.AnswerStream(ctx, retrievalseam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error starting stream: %v", err)
		}

		var chunks []qa.AnswerStreamChunk
		for chunk := range ch {
			chunks = append(chunks, chunk)
		}

		if len(chunks) != 1 || chunks[0].Type != qa.StreamChunkError {
			t.Fatalf("expected single error chunk, got %+v", chunks)
		}
		if chunks[0].Error == "" {
			t.Error("expected non-empty error message")
		}
	})
}

// errRetriever fails every retrieval with a canned error.
type errRetriever struct {
	err error
}

func (e *errRetriever) Retrieve(ctx context.Context, q retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	return nil, e.err
}
