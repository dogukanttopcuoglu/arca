package qa_test

import (
	"testing"

	"arca/internal/qa"
)

func TestDecideRetrievalRouting(t *testing.T) {
	t.Run("comparison sub-queries enable decomposition", func(t *testing.T) {
		analyzed := &qa.AnalyzedQuery{
			RawQuery:   "Compare Everyone Is a Creator with Beginner's Mind.",
			Intent:     "concept_lookup",
			SubQueries: []string{"Everyone Is a Creator", "Beginner's Mind."},
		}
		if got := qa.DecideRetrievalRouting(analyzed); !got.Decompose {
			t.Error("expected Decompose=true for comparison sub-queries")
		}
	})

	t.Run("non-comparison queries keep the Balanced path", func(t *testing.T) {
		analyzed := &qa.AnalyzedQuery{RawQuery: "What is creativity?", Intent: "concept_lookup"}
		if got := qa.DecideRetrievalRouting(analyzed); got.Decompose {
			t.Error("expected Decompose=false for a plain query")
		}
	})

	t.Run("nil analyzed query is a safe no-op", func(t *testing.T) {
		if got := qa.DecideRetrievalRouting(nil); got.Decompose {
			t.Error("expected Decompose=false for nil analysis")
		}
	})
}
