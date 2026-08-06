package eval_test

import (
	"os"
	"strings"
	"testing"

	"arca/internal/eval"
)

// TestGoldSetV1Lint is the always-on schema gate for the committed gold set:
// it must parse, validate, and be built exclusively from the real corpus
// (declared corpus fingerprint). No corpus or infrastructure is required.
func TestGoldSetV1Lint(t *testing.T) {
	f, err := os.Open("testdata/goldset_v1.json")
	if err != nil {
		t.Fatalf("gold set v1 missing: %v", err)
	}
	defer f.Close()

	gs, err := eval.LoadGoldSet(f)
	if err != nil {
		t.Fatalf("gold set v1 failed validation: %v", err)
	}

	if gs.Corpus.DocumentID != "rick-rubin" {
		t.Errorf("expected rick-rubin corpus, got %q", gs.Corpus.DocumentID)
	}
	if gs.Corpus.CorpusFingerprint == "" {
		t.Error("expected declared corpus fingerprint")
	}

	counts := map[string]int{}
	for _, q := range gs.Queries {
		counts[q.Intent]++
	}
	// v1 predates the heading category (ADR-0047, goldset v4): lint the six
	// categories v1 actually declares, so category additions don't break
	// older committed gold sets.
	v1Categories := []string{"single_fact", "concept", "procedural", "comparison", "entity", "abstention"}
	for _, intent := range v1Categories {
		if counts[intent] < 8 {
			t.Errorf("intent %q has %d queries, expected at least 8", intent, counts[intent])
		}
	}

	total := 0
	abstention := 0
	for _, q := range gs.Queries {
		total++
		if q.ExpectedNoEvidence {
			abstention++
			continue
		}
		for _, id := range q.ExpectedChunkIDs {
			if !strings.HasPrefix(id, gs.Corpus.DocumentID+"/") {
				t.Errorf("query %q references chunk outside the gold corpus: %q", q.ID, id)
			}
		}
	}
	if total < 50 {
		t.Errorf("expected at least 50 queries, got %d", total)
	}
	if abstention < 5 {
		t.Errorf("expected at least 5 abstention queries, got %d", abstention)
	}
}
