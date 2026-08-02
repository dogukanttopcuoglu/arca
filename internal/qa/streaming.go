package qa

import (
	"context"
	"fmt"
	"strings"

	llmprovider "arca/internal/llm/provider"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
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
	Type     StreamChunkType               `json:"type"`
	Content  string                        `json:"content,omitempty"`
	Verified *qaverification.VerifiedAnswer `json:"verified,omitempty"`
	Error    string                        `json:"error,omitempty"`
}

// StreamingAnswerEngine orchestrates interactive SSE token streaming and finalization verification.
type StreamingAnswerEngine struct {
	analyzer       QueryAnalyzer
	retriever      retrievalseam.Retriever
	contextBuilder qacontext.ContextBuilder
	promptBuilder  qaprompt.PromptBuilder
	llmProvider    llmprovider.LLMProvider
}

// NewStreamingAnswerEngine constructs a StreamingAnswerEngine instance.
func NewStreamingAnswerEngine(
	analyzer QueryAnalyzer,
	retriever retrievalseam.Retriever,
	ctxBuilder qacontext.ContextBuilder,
	promptBuilder qaprompt.PromptBuilder,
	llm llmprovider.LLMProvider,
) *StreamingAnswerEngine {
	if analyzer == nil {
		analyzer = NewRuleBasedAnalyzer()
	}
	if ctxBuilder == nil {
		ctxBuilder = qacontext.NewDefaultContextBuilder(nil, 4000)
	}
	if promptBuilder == nil {
		promptBuilder = qaprompt.NewRAGPromptBuilder()
	}
	return &StreamingAnswerEngine{
		analyzer:       analyzer,
		retriever:      retriever,
		contextBuilder: ctxBuilder,
		promptBuilder:  promptBuilder,
		llmProvider:    llm,
	}
}

// AnswerStream executes RAG retrieval, context building, token streaming, and stream finalization verification.
func (s *StreamingAnswerEngine) AnswerStream(ctx context.Context, query retrievalseam.RetrievalQuery) (<-chan AnswerStreamChunk, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}

	ch := make(chan AnswerStreamChunk, 50)

	go func() {
		defer close(ch)

		// 1. Analyze query
		_, err := s.analyzer.Analyze(ctx, query.QueryText)
		if err != nil {
			ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
			return
		}

		// 2. Execute retrieval if retriever present
		var searchResults []retrievalseam.SearchResult
		if s.retriever != nil {
			searchResults, _ = s.retriever.Retrieve(ctx, query)
		}

		// 3. Build context window
		win, err := s.contextBuilder.Build(ctx, searchResults)
		if err != nil {
			ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
			return
		}

		// 4. Build prompt message
		promptMsg, err := s.promptBuilder.Build(ctx, query.QueryText, win)
		if err != nil {
			ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
			return
		}

		// 5. Stream LLM tokens if provider present
		var fullText strings.Builder
		if s.llmProvider != nil {
			streamCh, err := s.llmProvider.Stream(ctx, promptMsg)
			if err != nil {
				ch <- AnswerStreamChunk{Type: StreamChunkError, Error: err.Error()}
				return
			}

			for chunk := range streamCh {
				if chunk.Error != nil {
					ch <- AnswerStreamChunk{Type: StreamChunkError, Error: chunk.Error.Error()}
					return
				}
				if chunk.Content != "" {
					fullText.WriteString(chunk.Content)
					ch <- AnswerStreamChunk{Type: StreamChunkToken, Content: chunk.Content}
				}
			}
		} else {
			fallback := "Mock streaming response content [Ref 1]."
			fullText.WriteString(fallback)
			ch <- AnswerStreamChunk{Type: StreamChunkToken, Content: fallback}
		}

		// 6. Run finalization verification
		verifier := qaverification.NewDefaultVerificationPipeline()
		verifiedAns, _ := verifier.Verify(ctx, fullText.String(), win)

		ch <- AnswerStreamChunk{
			Type:     StreamChunkVerification,
			Verified: verifiedAns,
		}
		ch <- AnswerStreamChunk{Type: StreamChunkDone}
	}()

	return ch, nil
}
