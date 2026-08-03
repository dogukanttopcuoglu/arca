package eval_test

import (
	"context"
	"strings"
	"testing"

	"arca/internal/eval"
	retrievalseam "arca/internal/retrieval/seam"
)

// signalGoldSet has one relevance query and one abstention query.
const signalGoldSet = `{
  "schema_version": "1",
  "corpus": {
    "document_id": "doc-1",
    "corpus_fingerprint": "bc1b8e4a7c0ce3149fb12980544f5bb2118685632b7139bc95edb218f0704a5e",
    "chunk_count": 3
  },
  "queries": [
    {
      "id": "q1",
      "intent": "concept",
      "query": "creative practice",
      "expected_chunk_ids": ["b"],
      "expected_no_evidence": false
    },
    {
      "id": "q2",
      "intent": "abstention",
      "query": "zzzqxx unknownwords",
      "expected_chunk_ids": [],
      "expected_no_evidence": true
    }
  ]
}`

// signalRetriever returns chunks with content for the relevance query and a
// flat score profile for the abstention query.
type signalRetriever struct{}

func (signalRetriever) Retrieve(ctx context.Context, q retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	if q.QueryText == "creative practice" {
		return []retrievalseam.SearchResult{
			{ChunkID: "b", Score: 0.9, ContentMarkdown: "Creative practice is a fundamental quality."},
			{ChunkID: "a", Score: 0.3, ContentMarkdown: "Unrelated filler text here."},
		}, nil
	}
	// Abstention query: flat score profile, no lexical overlap.
	return []retrievalseam.SearchResult{
		{ChunkID: "a", Score: 0.55, ContentMarkdown: "Unrelated filler text here."},
		{ChunkID: "b", Score: 0.54, ContentMarkdown: "Creative practice is a fundamental quality."},
		{ChunkID: "c", Score: 0.53, ContentMarkdown: "More unrelated content."},
	}, nil
}

func TestRunner_AbstentionSignals(t *testing.T) {
	gs, err := eval.LoadGoldSet(strings.NewReader(signalGoldSet))
	if err != nil {
		t.Fatalf("failed to load gold set: %v", err)
	}

	// Corpus with three documents: distinctive terms "practice"/"quality"
	// appear rarely; filler terms appear in all docs.
	corpus := func() ([]string, error) {
		return []string{
			"Creative practice is a fundamental quality.",
			"Unrelated filler text here.",
			"More unrelated content.",
		}, nil
	}

	runner := eval.New(signalRetriever{}, fakeFingerprintSource{
		hashes: []string{
			"3333333333333333333333333333333333333333333333333333333333333333",
			"1111111111111111111111111111111111111111111111111111111111111111",
			"2222222222222222222222222222222222222222222222222222222222222222",
		},
	}, eval.Options{Mode: retrievalseam.RetrievalDense, TopK: 5, CorpusTexts: corpus})

	report, err := runner.Run(context.Background(), gs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	q1 := report.PerQuery[0]
	if q1.Signals == nil {
		t.Fatal("expected abstention signals on q1")
	}
	// q1: distinctive terms "creative" (df 1), "practice" (df 1) — both in
	// the top result's content -> coverage 1.0. Score gap 0.9/0.3 = 3.0.
	if q1.Signals.LexicalCoverage != 1.0 {
		t.Errorf("expected q1 coverage 1.0, got %v", q1.Signals.LexicalCoverage)
	}
	if abs(q1.Signals.ScoreGap-3.0) > 1e-6 {
		t.Errorf("expected q1 score gap 3.0, got %v", q1.Signals.ScoreGap)
	}

	q2 := report.PerQuery[1]
	if q2.Signals == nil {
		t.Fatal("expected abstention signals on q2")
	}
	// q2: distinctive query terms "zzzqxx"/"unknownwords" (df 0) appear in no
	// retrieved content -> coverage 0.0. Flat scores -> gap ~1.02.
	if q2.Signals.LexicalCoverage != 0.0 {
		t.Errorf("expected q2 coverage 0.0, got %v", q2.Signals.LexicalCoverage)
	}
	if q2.Signals.ScoreGap > 1.1 {
		t.Errorf("expected q2 near-flat score gap, got %v", q2.Signals.ScoreGap)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
