package eval_test

import (
	"strings"
	"testing"

	"arca/internal/eval"
)

const validGoldSet = `{
  "schema_version": "1",
  "corpus": {
    "document_id": "rick-rubin",
    "corpus_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "chunk_count": 196
  },
  "queries": [
    {
      "id": "rr-001",
      "intent": "concept",
      "query": "What is the creative act about?",
      "expected_chunk_ids": ["rick-rubin/why-make-art/001"],
      "expected_sections": ["Why Make Art?"],
      "expected_no_evidence": false
    },
    {
      "id": "rr-002",
      "intent": "abstention",
      "query": "What is the capital of Atlantis?",
      "expected_chunk_ids": [],
      "expected_sections": [],
      "expected_no_evidence": true
    }
  ]
}`

func TestLoadGoldSet(t *testing.T) {
	t.Run("parses a valid gold set", func(t *testing.T) {
		gs, err := eval.LoadGoldSet(strings.NewReader(validGoldSet))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gs.Corpus.DocumentID != "rick-rubin" {
			t.Errorf("expected document id, got %q", gs.Corpus.DocumentID)
		}
		if len(gs.Queries) != 2 {
			t.Errorf("expected 2 queries, got %d", len(gs.Queries))
		}
		if !gs.Queries[1].ExpectedNoEvidence {
			t.Error("expected abstention query flagged")
		}
	})

	t.Run("rejects unknown intent category", func(t *testing.T) {
		bad := strings.Replace(validGoldSet, `"intent": "concept"`, `"intent": "bogus"`, 1)
		_, err := eval.LoadGoldSet(strings.NewReader(bad))
		if err == nil || !strings.Contains(err.Error(), "intent") {
			t.Errorf("expected intent validation error, got %v", err)
		}
	})

	t.Run("rejects abstention query with expected chunks", func(t *testing.T) {
		bad := strings.Replace(validGoldSet, `"expected_no_evidence": true`, `"expected_no_evidence": true, "expected_chunk_ids": ["x"]`, 1)
		_, err := eval.LoadGoldSet(strings.NewReader(bad))
		if err == nil {
			t.Error("expected error for abstention query declaring expected chunks")
		}
	})

	t.Run("rejects empty query text", func(t *testing.T) {
		bad := strings.Replace(validGoldSet, `"What is the capital of Atlantis?"`, `""`, 1)
		_, err := eval.LoadGoldSet(strings.NewReader(bad))
		if err == nil {
			t.Error("expected error for empty query text")
		}
	})

	t.Run("rejects duplicate query ids", func(t *testing.T) {
		bad := strings.Replace(validGoldSet, `"id": "rr-002"`, `"id": "rr-001"`, 1)
		_, err := eval.LoadGoldSet(strings.NewReader(bad))
		if err == nil {
			t.Error("expected error for duplicate query ids")
		}
	})

	t.Run("rejects missing corpus fingerprint", func(t *testing.T) {
		bad := strings.Replace(validGoldSet, `"corpus_fingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",`, ``, 1)
		_, err := eval.LoadGoldSet(strings.NewReader(bad))
		if err == nil {
			t.Error("expected error for missing corpus fingerprint")
		}
	})
}
