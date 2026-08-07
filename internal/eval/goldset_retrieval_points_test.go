package eval_test

import (
	"context"
	"strings"
	"testing"

	"arca/internal/eval"
	retrievalseam "arca/internal/retrieval/seam"
)

const retrievalPointsGoldSet = `{
  "schema_version": "1.2",
  "documents": [
    {"document_id": "book-a", "corpus_fingerprint": "fp-a", "chunk_count": 2}
  ],
  "queries": [
    {"id": "ov-01", "intent": "document_overview", "query": "What is this book about?",
     "expected_retrieval_points": ["book-a/document-profile/001", "book-a/intro/001"]},
    {"id": "ab-01", "intent": "abstention", "query": "What is the capital of Atlantis?", "expected_no_evidence": true}
  ]
}`

func TestGoldSet_ExpectedRetrievalPointsValidation(t *testing.T) {
	t.Run("retrieval-points-only queries load and validate", func(t *testing.T) {
		gs, err := eval.LoadGoldSet(strings.NewReader(retrievalPointsGoldSet))
		if err != nil {
			t.Fatalf("LoadGoldSet: %v", err)
		}
		q := gs.Queries[0]
		if q.Intent != "document_overview" {
			t.Fatalf("intent = %q", q.Intent)
		}
		ids := q.ExpectedIDs()
		if len(ids) != 2 || ids[0] != "book-a/document-profile/001" {
			t.Fatalf("ExpectedIDs = %v", ids)
		}
	})

	t.Run("declaring both expectation fields is rejected", func(t *testing.T) {
		both := strings.Replace(retrievalPointsGoldSet,
			`"expected_retrieval_points": ["book-a/document-profile/001", "book-a/intro/001"]`,
			`"expected_retrieval_points": ["book-a/document-profile/001"], "expected_chunk_ids": ["chk-1"]`, 1)
		if _, err := eval.LoadGoldSet(strings.NewReader(both)); err == nil {
			t.Fatal("expected both-fields rejection, got nil")
		}
	})

	t.Run("non-abstention query without expectations is rejected", func(t *testing.T) {
		none := strings.Replace(retrievalPointsGoldSet,
			`"expected_retrieval_points": ["book-a/document-profile/001", "book-a/intro/001"]`,
			`"expected_retrieval_points": []`, 1)
		if _, err := eval.LoadGoldSet(strings.NewReader(none)); err == nil {
			t.Fatal("expected no-expectations rejection, got nil")
		}
	})

	t.Run("abstention query cannot declare retrieval points", func(t *testing.T) {
		bad := strings.Replace(retrievalPointsGoldSet,
			`{"id": "ab-01", "intent": "abstention", "query": "What is the capital of Atlantis?", "expected_no_evidence": true}`,
			`{"id": "ab-01", "intent": "abstention", "query": "What is the capital of Atlantis?", "expected_no_evidence": true, "expected_retrieval_points": ["book-a/document-profile/001"]}`, 1)
		if _, err := eval.LoadGoldSet(strings.NewReader(bad)); err == nil {
			t.Fatal("expected abstention-with-points rejection, got nil")
		}
	})
}

func TestRunner_MetricsUseExpectedRetrievalPoints(t *testing.T) {
	gs, err := eval.LoadGoldSet(strings.NewReader(retrievalPointsGoldSet))
	if err != nil {
		t.Fatalf("LoadGoldSet: %v", err)
	}
	// The fake retriever surfaces the profile point first — the declared
	// expectation set must be evaluated as retrieval artifacts.
	ret := &fixedRetriever{ids: []string{"book-a/document-profile/001", "book-a/intro/001"}}
	liveFp := eval.ComputeFingerprint([]string{"h1", "h2"})
	gs.Documents[0].CorpusFingerprint = liveFp
	runner := eval.New(ret, fakeFingerprintSource{hashes: []string{"h1", "h2"}}, eval.Options{
		Mode: retrievalseam.RetrievalDense,
		TopK: 5,
	})

	report, err := runner.Run(context.Background(), gs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	qres := report.PerQuery[0]
	if qres.MRR != 1.0 {
		t.Fatalf("MRR = %.3f, want 1.0 (profile point first)", qres.MRR)
	}
	if qres.RecallAtK != 1.0 {
		t.Fatalf("recall@5 = %.3f, want 1.0", qres.RecallAtK)
	}
	if qres.NDCGAtK <= 0 {
		t.Fatalf("nDCG@5 = %.3f, want > 0", qres.NDCGAtK)
	}
}

// fixedRetriever returns a canned, ordered result list for every query.
type fixedRetriever struct {
	ids []string
}

func (f *fixedRetriever) Retrieve(ctx context.Context, q retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	results := make([]retrievalseam.SearchResult, len(f.ids))
	for i, id := range f.ids {
		results[i] = retrievalseam.SearchResult{ChunkID: id, Score: 1.0 - float32(i)*0.1}
	}
	return results, nil
}
