package eval_test

import (
	"context"
	"strings"
	"testing"

	"arca/internal/eval"
	retrievalseam "arca/internal/retrieval/seam"
)

// multiDocFingerprintSource returns per-document hashes for the two declared
// documents of the multi-document gold set.
type multiDocFingerprintSource struct{}

func (multiDocFingerprintSource) ContentHashes(documentID string) ([]string, error) {
	switch documentID {
	case "doc-a":
		return []string{"1111111111111111111111111111111111111111111111111111111111111111"}, nil
	case "doc-b":
		return []string{"2222222222222222222222222222222222222222222222222222222222222222"}, nil
	default:
		return nil, nil
	}
}

const multiDocGoldSet = `{
  "schema_version": "1.2",
  "documents": [
    {"document_id": "doc-a", "corpus_fingerprint": "3138bb9bc78df27c473ecfd1410f7bd45ebac1f59cf3ff9cfe4db77aab7aedd3", "chunk_count": 1},
    {"document_id": "doc-b", "corpus_fingerprint": "4f2e8d65483c641648cdb374ae9d8abd368d269e4ddffe092a8237b8162cddd6", "chunk_count": 1}
  ],
  "queries": [
    {"id": "m1", "intent": "comparison", "query": "How do doc-a and doc-b differ?", "expected_chunk_ids": ["a1", "b1"], "expected_sections": ["A", "B"], "expected_no_evidence": false}
  ]
}`

func TestRunner_MultiDocumentCorpus(t *testing.T) {
	gs, err := eval.LoadGoldSet(strings.NewReader(multiDocGoldSet))
	if err != nil {
		t.Fatalf("failed to load multi-document gold set: %v", err)
	}

	t.Run("per-document fingerprint mismatch hard-fails before queries", func(t *testing.T) {
		bogus := strings.Replace(multiDocGoldSet, "4f2e8d65483c641648cdb374ae9d8abd368d269e4ddffe092a8237b8162cddd6", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", 1)
		badGS, err := eval.LoadGoldSet(strings.NewReader(bogus))
		if err != nil {
			t.Fatalf("failed to load bogus gold set: %v", err)
		}
		ret := &fakeRetriever{}
		runner := eval.New(ret, multiDocFingerprintSource{}, eval.Options{
			Mode: retrievalseam.RetrievalDense,
			TopK: 5,
		})
		_, err = runner.Run(context.Background(), badGS)
		if err == nil || !strings.Contains(err.Error(), "doc-b") {
			t.Fatalf("expected per-document fingerprint mismatch for doc-b, got %v", err)
		}
		if ret.calls != 0 {
			t.Errorf("retriever must not run on fingerprint mismatch, got %d calls", ret.calls)
		}
	})

	t.Run("multi-document run records aggregate and per-document corpus", func(t *testing.T) {
		ret := &fakeRetriever{}
		runner := eval.New(ret, multiDocFingerprintSource{}, eval.Options{
			Mode: retrievalseam.RetrievalDense,
			TopK: 5,
		})
		report, err := runner.Run(context.Background(), gs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(report.Documents) != 2 {
			t.Fatalf("expected 2 document records, got %d", len(report.Documents))
		}
		if report.Corpus.ChunkCount != 2 {
			t.Errorf("expected aggregate chunk count 2, got %d", report.Corpus.ChunkCount)
		}
		if report.Documents[0].DocumentID != "doc-a" || report.Documents[1].DocumentID != "doc-b" {
			t.Errorf("unexpected document records: %+v", report.Documents)
		}
		if report.PerQuery[0].Intent != "comparison" {
			t.Errorf("expected comparison query recorded, got %q", report.PerQuery[0].Intent)
		}
	})
}
