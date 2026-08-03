package qa_test

import (
	"context"
	"testing"

	"arca/internal/qa"
)

func TestRuleBasedAnalyzer_Decomposition(t *testing.T) {
	ctx := context.Background()
	analyzer := qa.NewRuleBasedAnalyzer()

	t.Run("splits Compare X with Y into two sub-queries", func(t *testing.T) {
		got, err := analyzer.Analyze(ctx, "Compare Everyone Is a Creator with Beginner's Mind.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"Everyone Is a Creator", "Beginner's Mind."}
		if len(got.SubQueries) != 2 {
			t.Fatalf("expected 2 sub-queries, got %v", got.SubQueries)
		}
		for i := range want {
			if got.SubQueries[i] != want[i] {
				t.Errorf("expected sub-query %q, got %q", want[i], got.SubQueries[i])
			}
		}
	})

	t.Run("splits X vs Y", func(t *testing.T) {
		got, err := analyzer.Analyze(ctx, "Awareness vs Intention")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.SubQueries) != 2 {
			t.Fatalf("expected 2 sub-queries, got %v", got.SubQueries)
		}
		if got.SubQueries[0] != "Awareness" || got.SubQueries[1] != "Intention" {
			t.Errorf("unexpected sub-queries: %v", got.SubQueries)
		}
	})

	t.Run("splits difference-between forms", func(t *testing.T) {
		cases := map[string][]string{
			"Explain the difference between Seeds and Experimentation.": {"Seeds", "Experimentation."},
			"How do awareness and intention differ?":                     {"awareness", "intention"},
		}
		for query, want := range cases {
			got, err := analyzer.Analyze(ctx, query)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", query, err)
			}
			if len(got.SubQueries) != 2 {
				t.Errorf("query %q: expected 2 sub-queries, got %v", query, got.SubQueries)
				continue
			}
			for i := range want {
				if got.SubQueries[i] != want[i] {
					t.Errorf("query %q: expected sub-query %q, got %q", query, want[i], got.SubQueries[i])
				}
			}
		}
	})

	t.Run("single-intent queries produce no sub-queries", func(t *testing.T) {
		got, err := analyzer.Analyze(ctx, "What is the creative act about?")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.SubQueries != nil {
			t.Errorf("expected nil sub-queries, got %v", got.SubQueries)
		}
	})

	t.Run("empty query still errors", func(t *testing.T) {
		if _, err := analyzer.Analyze(ctx, ""); err == nil {
			t.Error("expected error for empty query")
		}
	})
}
