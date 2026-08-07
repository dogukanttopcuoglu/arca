package main

import (
	"strings"
	"testing"
)

func TestParseAskArgs(t *testing.T) {
	t.Run("query only", func(t *testing.T) {
		q, doc := parseAskArgs([]string{"What is creativity?"})
		if q != "What is creativity?" || doc != "" {
			t.Fatalf("got %q / %q", q, doc)
		}
	})

	t.Run("doc flag before query", func(t *testing.T) {
		q, doc := parseAskArgs([]string{"-doc", "rick", "What does the book say about tuning in?"})
		if q != "What does the book say about tuning in?" || doc != "rick" {
			t.Fatalf("got %q / %q", q, doc)
		}
	})

	t.Run("doc flag after query", func(t *testing.T) {
		q, doc := parseAskArgs([]string{"leverage points", "--doc", "systems"})
		if q != "leverage points" || doc != "systems" {
			t.Fatalf("got %q / %q", q, doc)
		}
	})

	t.Run("doc flag value is removed wherever it appears", func(t *testing.T) {
		q, doc := parseAskArgs([]string{"-doc", "rubin", "what", "does", "the", "book", "say"})
		if !strings.Contains(q, "what") || strings.Contains(q, "rubin") || doc != "rubin" {
			t.Fatalf("got %q / %q", q, doc)
		}
	})
}
