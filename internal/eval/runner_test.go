package eval_test

import (
	"context"
	"strings"
	"testing"

	"arca/internal/eval"
	indexingmodel "arca/internal/indexing/model"
	retrievalseam "arca/internal/retrieval/seam"
)

const runnerGoldSet = `{
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
      "query": "concept query one",
      "expected_chunk_ids": ["b", "d"],
      "expected_no_evidence": false
    },
    {
      "id": "q2",
      "intent": "entity",
      "query": "entity query two",
      "expected_chunk_ids": ["x"],
      "expected_no_evidence": false
    },
    {
      "id": "q3",
      "intent": "abstention",
      "query": "nothing matches this",
      "expected_chunk_ids": [],
      "expected_no_evidence": true
    }
  ]
}`

// fakeFingerprintSource returns the hashes whose sorted digest is the
// fingerprint declared above (1111, 2222, 3333).
type fakeFingerprintSource struct {
	hashes []string
}

func (f fakeFingerprintSource) ContentHashes(documentID string) ([]string, error) {
	return f.hashes, nil
}

// fakeRetriever returns canned results per query and records calls.
type fakeRetriever struct {
	calls int
}

func (f *fakeRetriever) Retrieve(ctx context.Context, q retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	f.calls++
	var ids []string
	switch q.QueryText {
	case "concept query one":
		ids = []string{"a", "b", "c", "d"}
	case "entity query two":
		ids = []string{"x", "y"}
	default:
		ids = nil
	}
	results := make([]retrievalseam.SearchResult, len(ids))
	for i, id := range ids {
		results[i] = retrievalseam.SearchResult{
			ChunkID:  id,
			Score:    1.0 - float32(i)*0.1,
			Metadata: indexingmodel.VectorMetadata{ChunkID: id},
		}
	}
	return results, nil
}

func newRunner(t *testing.T) (*eval.Runner, *eval.GoldSet, *fakeRetriever) {
	t.Helper()
	gs, err := eval.LoadGoldSet(strings.NewReader(runnerGoldSet))
	if err != nil {
		t.Fatalf("failed to load gold set: %v", err)
	}
	ret := &fakeRetriever{}
	runner := eval.New(ret, fakeFingerprintSource{
		hashes: []string{
			"3333333333333333333333333333333333333333333333333333333333333333",
			"1111111111111111111111111111111111111111111111111111111111111111",
			"2222222222222222222222222222222222222222222222222222222222222222",
		},
	}, eval.Options{
		Mode:              retrievalseam.RetrievalDense,
		TopK:              5,
		MinScore:          0,
		EmbeddingProvider: "Ollama",
		EmbeddingModel:    "nomic-embed-text:latest",
		Collection:        "arca_chunks",
	})
	return runner, gs, ret
}

func TestRunner(t *testing.T) {
	t.Run("hard-fails on fingerprint mismatch before any query", func(t *testing.T) {
		gs, err := eval.LoadGoldSet(strings.NewReader(runnerGoldSet))
		if err != nil {
			t.Fatalf("failed to load gold set: %v", err)
		}
		ret := &fakeRetriever{}
		runner := eval.New(ret, fakeFingerprintSource{
			hashes: []string{"deadbeef", "cafebabe"},
		}, eval.Options{Mode: retrievalseam.RetrievalDense, TopK: 5})

		_, err = runner.Run(context.Background(), gs)
		if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
			t.Fatalf("expected fingerprint mismatch error, got %v", err)
		}
		if ret.calls != 0 {
			t.Errorf("retriever must not be called on fingerprint mismatch, got %d calls", ret.calls)
		}
	})

	t.Run("computes per-query and aggregate metrics and writes a report", func(t *testing.T) {
		runner, gs, _ := newRunner(t)
		report, err := runner.Run(context.Background(), gs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(report.PerQuery) != 3 {
			t.Fatalf("expected 3 per-query results, got %d", len(report.PerQuery))
		}
		// q1: retrieved [a b c d], relevant {b d}: recall@5 1.0, precision@5 0.4, MRR 0.5
		if !approx(report.PerQuery[0].RecallAtK, 1.0) || !approx(report.PerQuery[0].PrecisionAtK, 0.4) || !approx(report.PerQuery[0].MRR, 0.5) {
			t.Errorf("q1 metrics wrong: %+v", report.PerQuery[0])
		}
		// q2: retrieved [x y], relevant {x}: recall 1.0, precision 0.2, MRR 1.0
		if !approx(report.PerQuery[1].RecallAtK, 1.0) || !approx(report.PerQuery[1].PrecisionAtK, 0.2) || !approx(report.PerQuery[1].MRR, 1.0) {
			t.Errorf("q2 metrics wrong: %+v", report.PerQuery[1])
		}

		// Aggregates over non-abstention queries: recall 1.0, precision 0.3, MRR 0.75
		if !approx(report.Metrics.RecallAtK, 1.0) || !approx(report.Metrics.PrecisionAtK, 0.3) || !approx(report.Metrics.MRR, 0.75) {
			t.Errorf("aggregate metrics wrong: %+v", report.Metrics)
		}
		// Abstention query returned zero results: no-evidence precision 1.0
		if !approx(report.Metrics.NoEvidencePrecision, 1.0) {
			t.Errorf("expected no-evidence precision 1.0, got %v", report.Metrics.NoEvidencePrecision)
		}

		// Report manifest fields
		if report.Retrieval.Mode != "Dense" {
			t.Errorf("expected mode Dense in report, got %q", report.Retrieval.Mode)
		}
		if report.Retrieval.TopK != 5 || report.Retrieval.EmbeddingModel != "nomic-embed-text:latest" {
			t.Errorf("retrieval config not recorded: %+v", report.Retrieval)
		}
		if report.Corpus.Fingerprint != "bc1b8e4a7c0ce3149fb12980544f5bb2118685632b7139bc95edb218f0704a5e" {
			t.Errorf("live fingerprint not recorded: %q", report.Corpus.Fingerprint)
		}
		if report.DurationMs < 0 || report.Timestamp.IsZero() {
			t.Error("expected timestamp and duration in report")
		}
	})
}
