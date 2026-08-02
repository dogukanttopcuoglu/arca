package context_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	qacontext "arca/internal/qa/context"
	retrievalseam "arca/internal/retrieval/seam"
)

func TestContextBuilder(t *testing.T) {
	ctx := context.Background()

	results := []retrievalseam.SearchResult{
		{
			ChunkID:         "chk-1",
			ContentMarkdown: "First chunk about creativity and discipline.",
			Score:           0.92,
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "doc-1",
				ChunkID:     "chk-1",
				SectionPath: "Introduction",
				PageNumbers: []int{12},
			},
		},
		{
			ChunkID:         "chk-2",
			ContentMarkdown: "Second chunk about flow state.",
			Score:           0.85,
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "doc-1",
				ChunkID:     "chk-2",
				SectionPath: "Flow",
				PageNumbers: []int{18},
			},
		},
		{
			// Duplicate of chk-1
			ChunkID:         "chk-1",
			ContentMarkdown: "First chunk about creativity and discipline.",
			Score:           0.90,
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "doc-1",
				ChunkID:     "chk-1",
				SectionPath: "Introduction",
				PageNumbers: []int{12},
			},
		},
	}

	tokenCounter := qacontext.NewSimpleTokenCounter()
	builder := qacontext.NewDefaultContextBuilder(tokenCounter, 1000)

	t.Run("builds prompt-ready ContextWindow with deduplication and citation markers", func(t *testing.T) {
		win, err := builder.Build(ctx, results)
		if err != nil {
			t.Fatalf("unexpected error building context window: %v", err)
		}

		if win == nil {
			t.Fatal("expected non-nil ContextWindow")
		}

		// Deduplication check: 3 input results -> 2 unique sources
		if len(win.Sources) != 2 {
			t.Fatalf("expected 2 unique sources after deduplication, got %d", len(win.Sources))
		}

		if win.Sources[0].CitationKey != "[Ref 1]" {
			t.Errorf("expected first citation key '[Ref 1]', got %q", win.Sources[0].CitationKey)
		}
		if win.Sources[1].CitationKey != "[Ref 2]" {
			t.Errorf("expected second citation key '[Ref 2]', got %q", win.Sources[1].CitationKey)
		}

		if win.TokenCount <= 0 {
			t.Errorf("expected TokenCount > 0, got %d", win.TokenCount)
		}
	})

	t.Run("truncates context when token budget is strictly limited", func(t *testing.T) {
		smallBuilder := qacontext.NewDefaultContextBuilder(tokenCounter, 15) // Small token budget
		win, err := smallBuilder.Build(ctx, results)
		if err != nil {
			t.Fatalf("unexpected error building small context: %v", err)
		}

		if len(win.Sources) >= 2 {
			t.Errorf("expected truncated sources count < 2 due to small token budget, got %d", len(win.Sources))
		}
	})
}
