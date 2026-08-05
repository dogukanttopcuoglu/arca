package qa_test

import (
	"context"
	"testing"

	llmprovider "arca/internal/llm/provider"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
	qaverification "arca/internal/qa/verification"
	"arca/internal/retrieval/seam"
)

// topKRecordingRetriever returns canned results and records the TopK of every
// retrieval call so tests can assert the evidence-budget override at the seam.
type topKRecordingRetriever struct {
	byQuery map[string][]seam.SearchResult
	topKs   []int
	queries []string
}

func (r *topKRecordingRetriever) Retrieve(ctx context.Context, q seam.RetrievalQuery) ([]seam.SearchResult, error) {
	r.topKs = append(r.topKs, q.TopK)
	r.queries = append(r.queries, q.QueryText)
	return r.byQuery[q.QueryText], nil
}

func TestAnswerEngine_ComparisonEvidenceBudget(t *testing.T) {
	ctx := context.Background()

	t.Run("comparison sub-queries use the calibrated TopK override", func(t *testing.T) {
		ret := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"Everyone Is a Creator": {sr("chk-a1"), sr("chk-a2"), sr("chk-a3"), sr("chk-a4")},
			"Beginner's Mind.":      {sr("chk-b1"), sr("chk-b2"), sr("chk-b3"), sr("chk-b4")},
		}}
		llm := &fakeLLM{content: "Both sides matter [Ref 1] [Ref 2] [Ref 3] [Ref 4] [Ref 5] [Ref 6] [Ref 7] [Ref 8]."}
		engine := newM6TestEngine(t, ret, qa.RetrievalRuntimeConfig{ComparisonTopK: 8}, llm)

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{
			QueryText: "Compare Everyone Is a Creator with Beginner's Mind.",
			TopK:      5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ans.Status != qaverification.StatusVerified {
			t.Fatalf("expected verified answer, got %q", ans.Status)
		}
		for i, tk := range ret.topKs {
			if tk != 8 {
				t.Errorf("sub-query %d: expected TopK 8, got %d", i, tk)
			}
		}
		if len(ret.queries) != 2 {
			t.Fatalf("expected 2 sub-query retrievals, got %d", len(ret.queries))
		}
		if len(ans.Citations) != 8 {
			t.Errorf("expected merged result trimmed to override 8, got %d citations", len(ans.Citations))
		}
	})

	t.Run("non-comparison queries keep the caller's TopK", func(t *testing.T) {
		ret := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"What is creativity?": {sr("chk-1"), sr("chk-2")},
		}}
		engine := newM6TestEngine(t, ret, qa.RetrievalRuntimeConfig{ComparisonTopK: 8}, &fakeLLM{content: "Grounded answer [Ref 1]."})

		_, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ret.topKs) != 1 || ret.topKs[0] != 5 {
			t.Errorf("expected single retrieval at caller TopK 5, got %v", ret.topKs)
		}
	})

	t.Run("zero config keeps legacy behavior", func(t *testing.T) {
		ret := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"Everyone Is a Creator": {sr("chk-a1"), sr("chk-a2")},
			"Beginner's Mind.":      {sr("chk-b1"), sr("chk-b2")},
		}}
		engine := newM6TestEngine(t, ret, qa.RetrievalRuntimeConfig{}, &fakeLLM{content: "Both sides [Ref 1]."})

		_, err := engine.Answer(ctx, seam.RetrievalQuery{
			QueryText: "Compare Everyone Is a Creator with Beginner's Mind.",
			TopK:      5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for i, tk := range ret.topKs {
			if tk != 5 {
				t.Errorf("sub-query %d: expected caller TopK 5 without config, got %d", i, tk)
			}
		}
	})
}

// newM6TestEngine builds an AnswerEngine with a configurable runtime config
// and injectable LLM; the gate stays nil (legacy test composition).
func newM6TestEngine(t *testing.T, retriever seam.Retriever, cfg qa.RetrievalRuntimeConfig, llm llmprovider.LLMProvider) *qa.AnswerEngine {
	t.Helper()
	return qa.NewAnswerEngine(
		qa.NewRuleBasedAnalyzer(),
		retriever,
		qacontext.NewDefaultContextBuilder(nil, 4000),
		qaprompt.NewRAGPromptBuilder(),
		llm,
		qaverification.NewDefaultVerificationPipeline(),
		nil,
		qa.WithRetrievalRuntimeConfig(cfg),
	)
}
