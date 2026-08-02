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
		keywords, err := extractor.Extract(ctx, nil, "en", nil)
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

		keywords, err := extractor.Extract(ctx, chunks, "en", nil)
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

		keywords, err := extractor.Extract(ctx, chunks, "en", nil)
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

		keywords, err := extractor.Extract(ctx, chunks, "en", nil)
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

		keywords, err := extractor.Extract(ctx, chunks, "tr", nil)
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

		keywords1, _ := extractor.Extract(ctx, chunks, "en", nil)
		keywords2, _ := extractor.Extract(ctx, chunks, "en", nil)

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

// Behavior Contract: Keyword extractor must suppress sub-tokens of extracted entity names.
// "def", "jam", "new", "york" must not appear in keywords when Def Jam Recordings and New York are entities.
func TestKeywordExtractor_FiltersEntityFragments(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedKeywordExtractor()

	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chunk-101",
			ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons. Creative expression is a fundamental human drive.",
		},
	}

	entities := []pdfmodel.Entity{
		{ID: "organization:def-jam-recordings", Name: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization},
		{ID: "location:new-york", Name: "New York", Type: pdfmodel.EntityTypeLocation},
		{ID: "person:rick-rubin", Name: "Rick Rubin", Type: pdfmodel.EntityTypePerson},
	}

	keywords, err := extractor.Extract(ctx, chunks, "en", entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entityFragments := []string{"def", "jam", "recordings", "new", "york", "rick", "rubin"}
	for _, kw := range keywords {
		for _, fragment := range entityFragments {
			if kw.Value == fragment {
				t.Errorf("BEHAVIOR CONTRACT VIOLATION: entity fragment %q leaked into keyword index", kw.Value)
			}
		}
	}

	// Semantic keywords must still be present
	foundSemantic := false
	semantic := map[string]bool{"creative": true, "expression": true, "founded": true, "alongside": true, "fundamental": true, "drive": true}
	for _, kw := range keywords {
		if semantic[kw.Value] {
			foundSemantic = true
		}
	}
	if !foundSemantic {
		t.Error("expected semantic keywords like 'creative' or 'expression' to remain after entity filtering")
	}
}
