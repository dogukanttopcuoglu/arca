package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestRuleBasedKeywordExtractor(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedKeywordExtractor()

	t.Run("empty document / chunks returns empty keywords", func(t *testing.T) {
		keywords, err := extractor.Extract(ctx, nil, "en")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keywords) != 0 {
			t.Errorf("expected 0 keywords, got %d", len(keywords))
		}
	})

	t.Run("single chunk extracts keywords with score and chunk_ids", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			{
				ChunkID:         "chunk-1",
				ContentMarkdown: "Vector search engine architecture and hybrid retrieval algorithms.",
			},
		}

		keywords, err := extractor.Extract(ctx, chunks, "en")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(keywords) == 0 {
			t.Fatal("expected non-empty keywords")
		}

		foundVector := false
		for _, kw := range keywords {
			if kw.Value == "vector" || kw.Value == "search" {
				foundVector = true
				if len(kw.ChunkIDs) == 0 || kw.ChunkIDs[0] != "chunk-1" {
					t.Errorf("expected chunk_ids to contain 'chunk-1', got %v", kw.ChunkIDs)
				}
				if kw.Source != pdfmodel.KeywordSourceRuleBased {
					t.Errorf("expected source 'rule_based', got %q", kw.Source)
				}
			}
		}
		if !foundVector {
			t.Error("expected keyword 'vector' or 'search' to be extracted")
		}
	})

	t.Run("multiple chunks merge duplicate keywords across chunks", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			{
				ChunkID:         "chunk-1",
				ContentMarkdown: "Database optimization and indexing strategies.",
			},
			{
				ChunkID:         "chunk-2",
				ContentMarkdown: "Indexing strategies for high throughput database queries.",
			},
		}

		keywords, err := extractor.Extract(ctx, chunks, "en")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, kw := range keywords {
			if kw.Value == "indexing" || kw.Value == "database" {
				if len(kw.ChunkIDs) < 2 {
					t.Errorf("expected merged chunk_ids for %q across 2 chunks, got %v", kw.Value, kw.ChunkIDs)
				}
			}
		}
	})

	t.Run("stopword filtering filters common English and Turkish stopwords", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			{
				ChunkID:         "chunk-1",
				ContentMarkdown: "The and is for with about bu ve veya için bir olarak",
			},
		}

		keywords, err := extractor.Extract(ctx, chunks, "en")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, kw := range keywords {
			if kw.Value == "the" || kw.Value == "and" || kw.Value == "for" {
				t.Errorf("stopword %q should have been filtered out", kw.Value)
			}
		}
	})

	t.Run("turkish language keyword extraction and unicode handling", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			{
				ChunkID:         "chunk-tr-1",
				ContentMarkdown: "Yazılım mimarisi, vektör veritabanı ve anlamsal arama performansı.",
			},
		}

		keywords, err := extractor.Extract(ctx, chunks, "tr")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		foundYazilim := false
		for _, kw := range keywords {
			if kw.Value == "yazılım" || kw.Value == "veritabanı" || kw.Value == "mimarisi" {
				foundYazilim = true
			}
		}
		if !foundYazilim {
			t.Error("expected Turkish unicode keywords to be correctly preserved")
		}
	})

	t.Run("deterministic output order and score ranking", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			{
				ChunkID:         "chunk-1",
				ContentMarkdown: "Architecture architecture architecture database database search",
			},
		}

		keywords1, _ := extractor.Extract(ctx, chunks, "en")
		keywords2, _ := extractor.Extract(ctx, chunks, "en")

		if len(keywords1) != len(keywords2) {
			t.Fatalf("nondeterministic keyword count: %d vs %d", len(keywords1), len(keywords2))
		}

		for i := range keywords1 {
			if keywords1[i].Value != keywords2[i].Value || keywords1[i].Score != keywords2[i].Score {
				t.Errorf("nondeterministic ordering at index %d: %v vs %v", i, keywords1[i], keywords2[i])
			}
		}

		// Architecture should be ranked #1 due to term frequency
		if len(keywords1) > 0 && keywords1[0].Value != "architecture" {
			t.Errorf("expected top keyword 'architecture', got %q", keywords1[0].Value)
		}
	})
}
