package eval_test

import (
	"math"
	"testing"

	"arca/internal/eval"
)

const eps = 1e-9

func approx(a, b float64) bool {
	return math.Abs(a-b) < eps
}

// Hand-computed fixtures: query with 5 retrieved docs where the relevant one
// is at position 3, and a query with no relevant docs.
func TestMetrics(t *testing.T) {
	t.Run("recall at k counts relevant retrieved over total relevant", func(t *testing.T) {
		retrieved := []string{"a", "b", "c", "d", "e"}
		relevant := []string{"c", "f"}

		if got := eval.RecallAtK(retrieved, relevant, 5); got != 0.5 {
			t.Errorf("expected recall@5 0.5, got %v", got)
		}
		if got := eval.RecallAtK(retrieved, relevant, 1); got != 0 {
			t.Errorf("expected recall@1 0, got %v", got)
		}
	})

	t.Run("precision at k counts relevant retrieved over k", func(t *testing.T) {
		retrieved := []string{"a", "b", "c", "d", "e"}
		relevant := []string{"c"}

		if got := eval.PrecisionAtK(retrieved, relevant, 5); got != 0.2 {
			t.Errorf("expected precision@5 0.2, got %v", got)
		}
		if got := eval.PrecisionAtK(retrieved, relevant, 3); got != 1.0/3.0 {
			t.Errorf("expected precision@3 1/3, got %v", got)
		}
	})

	t.Run("mrr is the reciprocal rank of the first relevant result", func(t *testing.T) {
		retrieved := []string{"a", "b", "c", "d"}
		relevant := []string{"c"}

		if got := eval.MRR(retrieved, relevant); got != 1.0/3.0 {
			t.Errorf("expected MRR 1/3, got %v", got)
		}

		if got := eval.MRR([]string{"x", "y"}, []string{"z"}); got != 0 {
			t.Errorf("expected MRR 0 when no relevant result, got %v", got)
		}

		if got := eval.MRR([]string{"c", "a"}, []string{"c", "a"}); got != 1.0 {
			t.Errorf("expected MRR 1 when top hit is relevant, got %v", got)
		}
	})

	t.Run("ndcg at k uses binary gains and ideal ordering", func(t *testing.T) {
		// Retrieved order: a(0) b(1) c(0) d(1); relevant = {b, d}, k=4.
		// DCG = 1/log2(3) + 1/log2(5) ; IDCG = 1/log2(2) + 1/log2(3)
		retrieved := []string{"a", "b", "c", "d"}
		relevant := []string{"b", "d"}

		got := eval.NDCGAtK(retrieved, relevant, 4)
		want := (1.0/1.584962500721156 + 1.0/2.321928094887362) / (1.0 + 1.0/1.584962500721156)
		if !approx(got, want) {
			t.Errorf("expected nDCG@4 %v, got %v", want, got)
		}

		// Perfect ordering yields nDCG 1.
		if got := eval.NDCGAtK([]string{"b", "d"}, relevant, 4); got != 1.0 {
			t.Errorf("expected nDCG 1 for perfect ordering, got %v", got)
		}

		// No relevant docs: nDCG is 0 (IDCG is 0).
		if got := eval.NDCGAtK([]string{"x", "y"}, []string{"z"}, 4); got != 0 {
			t.Errorf("expected nDCG 0 with no relevant docs, got %v", got)
		}
	})
}

func TestNoEvidencePrecision(t *testing.T) {
	// Abstention queries: retrieved counts [0, 3, 0] → 2 of 3 correctly empty.
	if got := eval.NoEvidencePrecision([]int{0, 3, 0}); got != 2.0/3.0 {
		t.Errorf("expected no-evidence precision 2/3, got %v", got)
	}
}
