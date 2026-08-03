package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestChunkStatisticsPass(t *testing.T) {
	ctx := context.Background()
	pass := enrichment.NewChunkStatisticsPass()

	t.Run("populates zero character_count and token_estimate from ContentMarkdown", func(t *testing.T) {
		content := "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons."
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{},
			Chunks: []pdfmodel.KnowledgeChunk{
				{ChunkID: "c1", ContentMarkdown: content, CharacterCount: 0, TokenEstimate: 0},
			},
		}

		if _, err := pass.Execute(ctx, input); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ch := input.Chunks[0]
		if ch.CharacterCount != len(content) {
			t.Errorf("expected character_count=%d, got %d", len(content), ch.CharacterCount)
		}
		if ch.TokenEstimate == 0 {
			t.Error("expected token_estimate > 0")
		}
		// token estimate should be chars/4
		expected := len(content) / 4
		if ch.TokenEstimate != expected {
			t.Errorf("expected token_estimate=%d (chars/4), got %d", expected, ch.TokenEstimate)
		}
	})

	t.Run("never overwrites values already set by chunking builder", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{},
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "c2",
					ContentMarkdown: "Some text.",
					CharacterCount:  9999, // pre-set by builder
					TokenEstimate:   8888, // pre-set by builder
				},
			},
		}

		if _, err := pass.Execute(ctx, input); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ch := input.Chunks[0]
		if ch.CharacterCount != 9999 {
			t.Errorf("expected builder value 9999 preserved, got %d", ch.CharacterCount)
		}
		if ch.TokenEstimate != 8888 {
			t.Errorf("expected builder value 8888 preserved, got %d", ch.TokenEstimate)
		}
	})

	t.Run("skips chunks with empty ContentMarkdown", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{},
			Chunks: []pdfmodel.KnowledgeChunk{
				{ChunkID: "c3", ContentMarkdown: "", CharacterCount: 0, TokenEstimate: 0},
			},
		}

		if _, err := pass.Execute(ctx, input); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ch := input.Chunks[0]
		if ch.CharacterCount != 0 || ch.TokenEstimate != 0 {
			t.Errorf("empty chunk should remain zero, got char=%d token=%d", ch.CharacterCount, ch.TokenEstimate)
		}
	})

	t.Run("minimum token estimate is 1 for very short content", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{},
			Chunks: []pdfmodel.KnowledgeChunk{
				{ChunkID: "c4", ContentMarkdown: "Hi", CharacterCount: 0, TokenEstimate: 0},
			},
		}

		if _, err := pass.Execute(ctx, input); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if input.Chunks[0].TokenEstimate < 1 {
			t.Error("token_estimate must be at least 1 for non-empty content")
		}
	})
}
