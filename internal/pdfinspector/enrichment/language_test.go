package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestLanguageDetectionPass(t *testing.T) {
	ctx := context.Background()
	pass := enrichment.NewLanguageDetectionPass()

	t.Run("detects Turkish language from page map text", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{},
			PageMap: []pdfmodel.PageMap{
				{PageNumber: 1, Markdown: "Bu belge Türkçe olarak kaleme alınmıştır. Yazılım mimarisi ve vektör arama motoru hakkında detaylar içermektedir."},
			},
		}

		_, err := pass.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if input.Metadata.Language != "tr" {
			t.Errorf("expected language 'tr', got %q", input.Metadata.Language)
		}
	})

	t.Run("detects English language from page map text", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{},
			PageMap: []pdfmodel.PageMap{
				{PageNumber: 1, Markdown: "This document describes the software architecture and vector search engine pipeline in detail."},
			},
		}

		_, err := pass.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if input.Metadata.Language != "en" {			t.Errorf("expected language 'en', got %q", input.Metadata.Language)
		}
	})
}
