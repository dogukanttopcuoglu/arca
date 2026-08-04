package qa

import (
	"context"
	"fmt"

	llmprovider "arca/internal/llm/provider"
	qacitation "arca/internal/qa/citation"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
	qaverification "arca/internal/qa/verification"
	"arca/internal/retrieval/seam"
)

// AnswerDraft encapsulates the intermediate pre-generation pipeline payload:
// the analyzed query and retrieved search results. A draft is not an answer
// until it has been generated and verified.
type AnswerDraft struct {
	QueryText     string              `json:"query_text"`
	AnalyzedQuery *AnalyzedQuery      `json:"analyzed_query,omitempty"`
	SearchResults []seam.SearchResult `json:"search_results"`
}

// AnswerMetadata carries provider-neutral metadata attached to an Answer
// (provider name, model identifier, token usage), keeping the domain model
// independent of any specific LLM provider.
type AnswerMetadata struct {
	Provider string                `json:"provider"`
	Model    string                `json:"model"`
	Usage    *llmprovider.LLMUsage `json:"usage,omitempty"`
}

// Answer is the final generated, evidence-verified response to a user query.
// It carries the answer text, verified citations, the verification report, an
// explicit VerificationStatus, and provider-agnostic metadata.
type Answer struct {
	Text         string                            `json:"text"`
	Citations    []qacitation.AnswerCitation       `json:"citations"`
	Verification qacitation.VerificationReport     `json:"verification"`
	Status       qaverification.VerificationStatus `json:"status"`
	Metadata     AnswerMetadata                    `json:"metadata"`
}

// AnswerEngine orchestrates the RAG pipeline stages using modular composition:
// Analyze -> Retrieve -> ContextBuilder -> EvidenceGate -> PromptBuilder -> LLM.Generate -> Verification.
type AnswerEngine struct {
	analyzer       QueryAnalyzer
	retriever      seam.Retriever
	contextBuilder qacontext.ContextBuilder
	promptBuilder  qaprompt.PromptBuilder
	llmProvider    llmprovider.LLMProvider
	verifier       qaverification.VerificationPipeline
	evidenceGate   EvidenceGate
}

// NewAnswerEngine constructs an AnswerEngine instance. Nil seams fall back to
// working defaults (rule-based analyzer, default context builder, RAG prompt
// builder, mock LLM provider, default verification pipeline) so the engine is
// usable offline; production composition supplies the real seams. A nil
// evidenceGate disables pre-generation semantic abstention and is reserved
// for tests and explicitly legacy/offline composition — every production
// composition root must supply a real gate (ADR-0030).
func NewAnswerEngine(
	analyzer QueryAnalyzer,
	retriever seam.Retriever,
	ctxBuilder qacontext.ContextBuilder,
	promptBuilder qaprompt.PromptBuilder,
	llm llmprovider.LLMProvider,
	verifier qaverification.VerificationPipeline,
	evidenceGate EvidenceGate,
) *AnswerEngine {
	if analyzer == nil {
		analyzer = NewRuleBasedAnalyzer()
	}
	if ctxBuilder == nil {
		ctxBuilder = qacontext.NewDefaultContextBuilder(nil, 4000)
	}
	if promptBuilder == nil {
		promptBuilder = qaprompt.NewRAGPromptBuilder()
	}
	if llm == nil {
		llm = llmprovider.NewMockLLMProvider("mock-llm-provider", "mock-model-v1")
	}
	if verifier == nil {
		verifier = qaverification.NewDefaultVerificationPipeline()
	}
	return &AnswerEngine{
		analyzer:       analyzer,
		retriever:      retriever,
		contextBuilder: ctxBuilder,
		promptBuilder:  promptBuilder,
		llmProvider:    llm,
		verifier:       verifier,
		evidenceGate:   evidenceGate,
	}
}

// evaluateGate runs the evidence gate with one bounded retry (ADR-0034).
// supported and unsupported decisions return immediately; operational
// failures (errors or EvidenceGateFailed decisions) are retried once and
// surface as a typed EvidenceGateError after exhaustion.
func (e *AnswerEngine) evaluateGate(ctx context.Context, query string, win *qacontext.ContextWindow) (EvidenceDecision, error) {
	var lastErr error
	for attempt := 1; attempt <= MaxGateAttempts; attempt++ {
		decision, err := e.evidenceGate.Evaluate(ctx, query, win)
		if err != nil || decision == EvidenceGateFailed {
			lastErr = err
			continue
		}
		return decision, nil
	}
	return EvidenceGateFailed, &EvidenceGateError{Attempts: MaxGateAttempts, Cause: lastErr}
}

// Answer executes the full RAG pipeline and returns the final Answer.
// When retrieval yields no sources, generation is skipped entirely and the
// Answer carries the no_evidence status.
func (e *AnswerEngine) Answer(ctx context.Context, query seam.RetrievalQuery) (*Answer, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}

	draft := &AnswerDraft{QueryText: query.QueryText}

	analyzed, err := e.analyzer.Analyze(ctx, query.QueryText)
	if err != nil {
		return nil, fmt.Errorf("query analysis failed: %w", err)
	}
	draft.AnalyzedQuery = analyzed

	if e.retriever != nil {
		// M5 orchestration (ADR-0032): comparison queries detected by the
		// existing analyzer signal retrieve each sub-query and merge
		// deterministically; everything else takes the direct Balanced path.
		if DecideRetrievalRouting(analyzed).Decompose {
			var lists [][]seam.SearchResult
			for _, sub := range analyzed.SubQueries {
				subQuery := query
				subQuery.QueryText = sub
				results, err := e.retriever.Retrieve(ctx, subQuery)
				if err != nil {
					return nil, fmt.Errorf("retrieval stage failed for sub-query %q: %w", sub, err)
				}
				if len(results) > 0 {
					lists = append(lists, results)
				}
			}
			draft.SearchResults = seam.MergeRankedLists(lists, query.TopK)
		} else {
			draft.SearchResults, err = e.retriever.Retrieve(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("retrieval stage failed: %w", err)
			}
		}
	}

	if len(draft.SearchResults) == 0 {
		return &Answer{
			Text:   "The retrieved sources do not cover this query, so no grounded answer can be provided.",
			Status: qaverification.StatusNoEvidence,
		}, nil
	}
	win, err := e.contextBuilder.Build(ctx, draft.SearchResults)
	if err != nil {
		return nil, fmt.Errorf("context assembly failed: %w", err)
	}

	// Pre-generation semantic abstention (ADR-0034): the gate evaluates the
	// original query against the exact context that would reach generation.
	// Unsupported context abstains without an LLM call; operational gate
	// failures retry once, then fail closed with a typed error. A nil gate
	// preserves legacy behavior for tests and offline composition.
	if e.evidenceGate != nil {
		decision, gateErr := e.evaluateGate(ctx, query.QueryText, win)
		if gateErr != nil {
			return nil, gateErr
		}
		if decision == EvidenceUnsupported {
			return &Answer{
				Text:   "The retrieved sources do not cover this query, so no grounded answer can be provided.",
				Status: qaverification.StatusNoEvidence,
			}, nil
		}
	}

	promptMsg, err := e.promptBuilder.Build(ctx, query.QueryText, win)
	if err != nil {
		return nil, fmt.Errorf("prompt assembly failed: %w", err)
	}

	llmResp, err := e.llmProvider.Generate(ctx, promptMsg)
	if err != nil {
		return nil, fmt.Errorf("answer generation failed: %w", err)
	}

	verified, err := e.verifier.Verify(ctx, llmResp.Content, win)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	return &Answer{
		Text:         verified.Text,
		Citations:    verified.Citations,
		Verification: verified.Verification,
		Status:       verified.Status,
		Metadata: AnswerMetadata{
			Provider: llmResp.Provider,
			Model:    llmResp.Model,
			Usage:    &llmResp.TokenUsage,
		},
	}, nil
}
