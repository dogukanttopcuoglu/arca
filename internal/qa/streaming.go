package qa

import (
	"context"
	"fmt"
	"strings"

	llmprovider "arca/internal/llm/provider"
	qaverification "arca/internal/qa/verification"
	retrievalseam "arca/internal/retrieval/seam"
)

// StreamChunkType specifies SSE stream payload type.
type StreamChunkType string

const (
	StreamChunkToken        StreamChunkType = "token"
	StreamChunkVerification StreamChunkType = "verification"
	StreamChunkDone         StreamChunkType = "done"
	StreamChunkError        StreamChunkType = "error"
)

// AnswerStreamChunk models a single Server-Sent Event (SSE) payload chunk.
type AnswerStreamChunk struct {
	Type     StreamChunkType                `json:"type"`
	Content  string                         `json:"content,omitempty"`
	Verified *qaverification.VerifiedAnswer `json:"verified,omitempty"`
	Error    string                         `json:"error,omitempty"`
}

// AnswerStream runs the shared pre-generation pipeline, then streams LLM
// tokens and a final verification chunk carrying the runtime verifier's
// decision. An abstention streams no tokens: the verification chunk carries
// the same no_evidence answer as the synchronous path. A nil LLM provider
// falls back to a canned token so offline callers still observe the full
// stream lifecycle, matching the legacy seam's behavior.
func (e *AnswerEngine) AnswerStream(ctx context.Context, query retrievalseam.RetrievalQuery) (<-chan AnswerStreamChunk, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}

	ch := make(chan AnswerStreamChunk, 50)

	go func() {
		defer close(ch)

		prepared, err := e.prepare(ctx, query)
		if err != nil {
			ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
			return
		}

		if prepared.abstain {
			ch <- AnswerStreamChunk{
				Type: StreamChunkVerification,
				Verified: &qaverification.VerifiedAnswer{
					Text:   abstainText,
					Status: qaverification.StatusNoEvidence,
				},
			}
			ch <- AnswerStreamChunk{Type: StreamChunkDone}
			return
		}

		promptMsg, err := e.promptBuilder.Build(ctx, query.QueryText, prepared.window)
		if err != nil {
			ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
			return
		}

		var (
			fullText strings.Builder
			stream   <-chan llmprovider.StreamChunk
		)
		if e.llmProvider != nil {
			stream, err = e.llmProvider.Stream(ctx, promptMsg)
			if err != nil {
				ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
				return
			}
		}
		if stream == nil {
			fallback := "Mock streaming response content [Ref 1]."
			fullText.WriteString(fallback)
			ch <- AnswerStreamChunk{Type: StreamChunkToken, Content: fallback}
		} else {
			for chunk := range stream {
				if chunk.Error != nil {
					ch <- AnswerStreamChunk{Type: StreamChunkError, Error: chunk.Error.Error()}
					return
				}
				if chunk.Content != "" {
					fullText.WriteString(chunk.Content)
					ch <- AnswerStreamChunk{Type: StreamChunkToken, Content: chunk.Content}
				}
			}
		}

		verified, err := e.verifier.Verify(ctx, fullText.String(), prepared.window)
		if err != nil {
			ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
			return
		}

		ch <- AnswerStreamChunk{Type: StreamChunkVerification, Verified: verified}
		ch <- AnswerStreamChunk{Type: StreamChunkDone}
	}()

	return ch, nil
}
