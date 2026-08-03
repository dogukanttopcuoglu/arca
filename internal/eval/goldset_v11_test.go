package eval_test

import (
	"os"
	"strings"
	"testing"

	"arca/internal/eval"
)

// TestGoldSetV11Lint gates the hardened gold set: same corpus fingerprint as
// v1, extended comparison queries in non-patterned linguistic forms.
func TestGoldSetV11Lint(t *testing.T) {
	f, err := os.Open("testdata/goldset_v1_1.json")
	if err != nil {
		t.Fatalf("gold set v1.1 missing: %v", err)
	}
	defer f.Close()

	gs, err := eval.LoadGoldSet(f)
	if err != nil {
		t.Fatalf("gold set v1.1 failed validation: %v", err)
	}

	if gs.Corpus.CorpusFingerprint != "51b1909e1a2639b3114eb4e9afaeeafa58938cbdb0d195392dc65d06ac0b483d" {
		t.Error("v1.1 must declare the same corpus fingerprint as v1")
	}

	comparisons := []eval.GoldQuery{}
	for _, q := range gs.Queries {
		if q.Intent == "comparison" {
			comparisons = append(comparisons, q)
		}
	}
	if len(comparisons) < 12 {
		t.Errorf("expected at least 12 comparison queries, got %d", len(comparisons))
	}

	// Hardened forms must be present (non-"Compare X with Y" patterns).
	forms := []string{"differ", "distinguishes", "difference between", "contrast"}
	found := map[string]bool{}
	for _, q := range comparisons {
		lower := strings.ToLower(q.Query)
		for _, f := range forms {
			if strings.Contains(lower, f) {
				found[f] = true
			}
		}
	}
	for _, f := range forms {
		if !found[f] {
			t.Errorf("missing hardened comparison form %q", f)
		}
	}
}
