package qa

import (
	"context"
	"fmt"

	"arca/internal/retrieval/seam"
)

// AnswerDraft encapsulates intermediate QA pipeline results before evidence verification.
type AnswerDraft struct {
	QueryText     string              `json:"query_text"`
	AnalyzedQuery *AnalyzedQuery      `json:"analyzed_query,omitempty"`
	SearchResults []seam.SearchResult `json:"search_results"`
}

// AnswerEngine orchestrates RAG pipeline stages using modular composition.
type AnswerEngine struct {
	analyzer  QueryAnalyzer
	retriever seam.Retriever
}

// NewAnswerEngine constructs an AnswerEngine instance.
func NewAnswerEngine(analyzer QueryAnalyzer, retriever seam.Retriever, eval unusedInterface, ctxBuilder unusedInterface, llm unusedInterface) *AnswerEngine {
	_ = eval
	_ = ctxBuilder
	_ = llm

	if analyzer == nil {
		analyzer = NewRuleBasedAnalyzer()
	}
	return &AnswerEngine{
		analyzer:  analyzer,
		retriever: retriever,
	}
}

type unusedInterface interface{}

// Answer executes the QA orchestration pipeline across QueryAnalyzer and Retriever.
func (e *AnswerEngine) Answer(ctx context.Context, query seam.RetrievalQuery) (*AnswerDraft, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}

	analyzed, err := e.analyzer.Analyze(ctx, query.QueryText)
	if err != nil {
		return nil, fmt.Errorf("query analysis failed: %w", err)
	}

	var results []seam.SearchResult
	if e.retriever != nil {
		results, err = e.retriever.Retrieve(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("retrieval stage failed: %w", err)
		}
	}

	return &AnswerDraft{
		QueryText:     query.QueryText,
		AnalyzedQuery: analyzed,
		SearchResults: results,
	}, nil
}
