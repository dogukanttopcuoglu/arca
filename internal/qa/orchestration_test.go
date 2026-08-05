package qa_test

import (
	"testing"

	"arca/internal/qa"
)

func TestAnalyzeIntentHint(t *testing.T) {
	t.Run("comparison sub-queries produce a comparison hint", func(t *testing.T) {
		analyzed := &qa.AnalyzedQuery{
			RawQuery:   "Compare Everyone Is a Creator with Beginner's Mind.",
			Intent:     "concept_lookup",
			SubQueries: []string{"Everyone Is a Creator", "Beginner's Mind."},
		}
		hint := qa.AnalyzeIntentHint(analyzed)
		if hint.Intent != qa.HintIntentComparison || !hint.Decompose {
			t.Errorf("expected comparison hint with Decompose, got %+v", hint)
		}
		if hint.Source != qa.HintSourceRuleBased {
			t.Errorf("expected rule_based source, got %q", hint.Source)
		}
	})

	t.Run("plain queries produce an other hint without decomposition", func(t *testing.T) {
		analyzed := &qa.AnalyzedQuery{RawQuery: "What is creativity?", Intent: "concept_lookup"}
		hint := qa.AnalyzeIntentHint(analyzed)
		if hint.Intent != qa.HintIntentOther || hint.Decompose {
			t.Errorf("expected other hint without Decompose, got %+v", hint)
		}
	})

	t.Run("nil analyzed query is a safe no-op", func(t *testing.T) {
		hint := qa.AnalyzeIntentHint(nil)
		if hint.Intent != qa.HintIntentOther || hint.Decompose {
			t.Errorf("expected other hint for nil analysis, got %+v", hint)
		}
	})
}

func TestDecideRetrievalRouting(t *testing.T) {
	cfg := qa.RetrievalRuntimeConfig{ComparisonTopK: 8}

	t.Run("comparison hints enable decomposition", func(t *testing.T) {
		hint := qa.IntentHint{Intent: qa.HintIntentComparison, Decompose: true, Source: qa.HintSourceRuleBased}
		if got := qa.DecideRetrievalRouting(hint, cfg); !got.Decompose {
			t.Error("expected Decompose=true for comparison hint")
		}
	})

	t.Run("non-comparison hints keep the Balanced path", func(t *testing.T) {
		hint := qa.IntentHint{Intent: qa.HintIntentOther, Source: qa.HintSourceRuleBased}
		if got := qa.DecideRetrievalRouting(hint, cfg); got.Decompose {
			t.Error("expected Decompose=false for a plain hint")
		}
	})

	t.Run("comparison hints receive the calibrated TopK override", func(t *testing.T) {
		hint := qa.IntentHint{Intent: qa.HintIntentComparison, Decompose: true, Source: qa.HintSourceRuleBased}
		got := qa.DecideRetrievalRouting(hint, cfg)
		if got.TopKOverride != 8 {
			t.Errorf("expected comparison TopK override 8, got %d", got.TopKOverride)
		}
	})

	t.Run("non-comparison hints never override TopK", func(t *testing.T) {
		hint := qa.IntentHint{Intent: qa.HintIntentOther, Source: qa.HintSourceRuleBased}
		got := qa.DecideRetrievalRouting(hint, cfg)
		if got.TopKOverride != 0 {
			t.Errorf("expected no TopK override for plain hints, got %d", got.TopKOverride)
		}
	})

	t.Run("zero config means no override even for comparison", func(t *testing.T) {
		hint := qa.IntentHint{Intent: qa.HintIntentComparison, Decompose: true, Source: qa.HintSourceRuleBased}
		got := qa.DecideRetrievalRouting(hint, qa.RetrievalRuntimeConfig{})
		if got.TopKOverride != 0 {
			t.Errorf("expected no override when config is unset, got %d", got.TopKOverride)
		}
	})
}
