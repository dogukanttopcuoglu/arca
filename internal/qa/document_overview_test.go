package qa_test

import (
	"context"
	"strings"
	"testing"

	llmprovider "arca/internal/llm/provider"
	"arca/internal/qa"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
	qaverification "arca/internal/qa/verification"
	"arca/internal/retrieval/seam"
)

func TestRuleBasedAnalyzer_DocumentOverviewDetection(t *testing.T) {
	analyzer := qa.NewRuleBasedAnalyzer()
	analyze := func(q string) string {
		t.Helper()
		a, err := analyzer.Analyze(context.Background(), q)
		if err != nil {
			t.Fatalf("Analyze(%q): %v", q, err)
		}
		return a.Intent
	}

	overview := []string{
		// EN
		"What is this book about?",
		"What is the book about?",
		"What does this book cover?",
		"Summarize this book",
		"Summary of the book",
		"What is the main idea?",
		"Who is the author?",
		"Who wrote this book?",
		// TR
		"Bu kitap ne anlatıyor?",
		"kitap ne hakkında",
		"Kitabı özetle",
		"özetle",
		"Ana fikri nedir?",
		"Yazar kim?",
		"Yazarı kim?",
		"Bu kitabı kim yazdı?",
	}
	for _, q := range overview {
		if got := analyze(q); got != "document_overview" {
			t.Errorf("query %q: intent = %q, want document_overview", q, got)
		}
	}

	notOverview := []string{
		"What does the book say about beginner's mind?",
		"What does the book say about the author?",
		"Who is Rick Rubin?",
		"Explain section three",
		"What is the summary of chapter 2?",
		"Bölüm 3'ün özeti nedir?",
		"Özet the concept of bounded contexts",
		"How does CQRS work?",
		"Compare X with Y",
	}
	for _, q := range notOverview {
		if got := analyze(q); got == "document_overview" {
			t.Errorf("query %q: intent = %q, must NOT be document_overview", q, got)
		}
	}
}

func TestIntentHint_DocumentOverview(t *testing.T) {
	analyzer := qa.NewRuleBasedAnalyzer()

	t.Run("overview intent maps to the document_overview hint", func(t *testing.T) {
		a, err := analyzer.Analyze(context.Background(), "Bu kitap ne anlatıyor?")
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		hint := qa.AnalyzeIntentHint(a)
		if hint.Intent != qa.HintIntentDocumentOverview {
			t.Fatalf("hint = %q, want %q", hint.Intent, qa.HintIntentDocumentOverview)
		}
		if hint.Source != qa.HintSourceRuleBased {
			t.Fatalf("hint source = %q, want rule_based", hint.Source)
		}
	})

	t.Run("entity and comparison hints stay unchanged", func(t *testing.T) {
		a, err := analyzer.Analyze(context.Background(), "What does the book say about World Bank?")
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		if hint := qa.AnalyzeIntentHint(a); hint.Intent != qa.HintIntentEntity {
			t.Fatalf("entity hint = %q", hint.Intent)
		}
		a, err = analyzer.Analyze(context.Background(), "Compare X with Y.")
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		if hint := qa.AnalyzeIntentHint(a); hint.Intent != qa.HintIntentComparison {
			t.Fatalf("comparison hint = %q", hint.Intent)
		}
	})
}

func TestRetrievalOrchestrator_DocumentOverview(t *testing.T) {
	t.Run("document_overview hint opens the overview decision", func(t *testing.T) {
		d := qa.DecideRetrievalRouting(qa.IntentHint{Intent: qa.HintIntentDocumentOverview}, qa.RetrievalRuntimeConfig{})
		if !d.DocumentOverview {
			t.Fatalf("decision = %+v, want DocumentOverview true", d)
		}
		if d.Decompose || d.UseGraph {
			t.Fatalf("overview decision must not decompose or open the graph: %+v", d)
		}
	})

	t.Run("comparison, entity, other decisions stay byte-identical", func(t *testing.T) {
		cfg := qa.RetrievalRuntimeConfig{ComparisonTopK: 8, GraphWeight: 1.0}
		cmp := qa.DecideRetrievalRouting(qa.IntentHint{Intent: qa.HintIntentComparison}, cfg)
		if !cmp.Decompose || cmp.DocumentOverview || cmp.UseGraph || cmp.TopKOverride != 8 {
			t.Fatalf("comparison decision drifted: %+v", cmp)
		}
		ent := qa.DecideRetrievalRouting(qa.IntentHint{Intent: qa.HintIntentEntity}, cfg)
		if !ent.UseGraph || ent.DocumentOverview || ent.Decompose {
			t.Fatalf("entity decision drifted: %+v", ent)
		}
		other := qa.DecideRetrievalRouting(qa.IntentHint{Intent: qa.HintIntentOther}, cfg)
		if other != (qa.RetrievalDecision{}) {
			t.Fatalf("other decision drifted: %+v", other)
		}
	})
}

func TestAnswerEngine_DocumentOverviewRouting(t *testing.T) {
	ctx := context.Background()

	t.Run("overview queries execute through the injected overview retriever", func(t *testing.T) {
		base := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"Bu kitap ne anlatıyor?": {sr("chk-base")},
		}}
		overview := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"Bu kitap ne anlatıyor?": {sr("chk-profile")},
		}}
		engine := newM9OverviewEngine(t, base, overview, &fakeLLM{content: "Overview [Ref 1]."})

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "Bu kitap ne anlatıyor?", TopK: 5})
		if err != nil {
			t.Fatalf("Answer: %v", err)
		}
		if len(base.topKs) != 0 {
			t.Errorf("base retriever must be untouched for overview queries, got %v", base.topKs)
		}
		if len(overview.topKs) != 1 {
			t.Errorf("overview retriever must serve the query, got %v", overview.topKs)
		}
		if len(ans.Citations) != 1 || ans.Citations[0].ChunkID != "chk-profile" {
			t.Errorf("expected profile citation, got %+v", ans.Citations)
		}
	})

	t.Run("non-overview queries keep the base retriever even with the overview injected", func(t *testing.T) {
		base := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"What is creativity?": {sr("chk-dense")},
		}}
		overview := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"What is creativity?": {sr("chk-profile")},
		}}
		engine := newM9OverviewEngine(t, base, overview, &fakeLLM{content: "Grounded [Ref 1]."})

		_, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "What is creativity?", TopK: 5})
		if err != nil {
			t.Fatalf("Answer: %v", err)
		}
		if len(base.topKs) != 1 || len(overview.topKs) != 0 {
			t.Errorf("expected base retriever for non-overview, base=%v overview=%v", base.topKs, overview.topKs)
		}
	})

	t.Run("nil overview retriever keeps current behavior", func(t *testing.T) {
		base := &topKRecordingRetriever{byQuery: map[string][]seam.SearchResult{
			"Bu kitap ne anlatıyor?": {sr("chk-base")},
		}}
		engine := newM9OverviewEngine(t, base, nil, &fakeLLM{content: "Grounded [Ref 1]."})

		ans, err := engine.Answer(ctx, seam.RetrievalQuery{QueryText: "Bu kitap ne anlatıyor?", TopK: 5})
		if err != nil {
			t.Fatalf("Answer: %v", err)
		}
		if len(base.topKs) != 1 {
			t.Fatalf("nil overview retriever must fall back to the base retriever, got %v", base.topKs)
		}
		if !strings.Contains(ans.Text, "Grounded") {
			t.Fatalf("unexpected answer text: %s", ans.Text)
		}
	})
}

// newM9OverviewEngine builds an AnswerEngine with a base retriever and an
// optional overview retriever (M9, ADR-0048); the evidence gate stays nil
// (legacy test composition, mirroring newM6TestEngine).
func newM9OverviewEngine(t *testing.T, retriever, overview seam.Retriever, llm llmprovider.LLMProvider) *qa.AnswerEngine {
	t.Helper()
	opts := []qa.AnswerEngineOption{}
	if overview != nil {
		opts = append(opts, qa.WithDocumentOverviewRetriever(overview))
	}
	return qa.NewAnswerEngine(
		qa.NewRuleBasedAnalyzer(),
		retriever,
		qacontext.NewDefaultContextBuilder(nil, 4000),
		qaprompt.NewRAGPromptBuilder(),
		llm,
		qaverification.NewDefaultVerificationPipeline(),
		nil,
		opts...,
	)
}
