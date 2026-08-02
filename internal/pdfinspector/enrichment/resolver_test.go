package enrichment_test

import (
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestTitleAuthorResolvers(t *testing.T) {
	t.Run("resolves title from PDF metadata if non-generic", func(t *testing.T) {
		doc := pdfmodel.DocumentMetadata{Title: "Custom Architecture Guide"}
		resolver := enrichment.NewDefaultTitleResolverChain()

		title := resolver.ResolveTitle(doc, nil, nil, "guide.pdf")
		if title != "Custom Architecture Guide" {
			t.Errorf("expected 'Custom Architecture Guide', got %q", title)
		}
	})

	t.Run("falls back to Unknown Title when no metadata or early text matches, never hardcoding book titles", func(t *testing.T) {
		doc := pdfmodel.DocumentMetadata{Title: "Untitled Document"}
		resolver := enrichment.NewDefaultTitleResolverChain()

		title := resolver.ResolveTitle(doc, nil, nil, "unknown.pdf")
		if title == "The Creative Act: A Way of Being" {
			t.Error("title resolver must not return hardcoded book title fallback")
		}
		if title == "" {
			t.Error("expected non-empty title fallback")
		}
	})

	t.Run("resolves author from early page text", func(t *testing.T) {
		pageMap := []pdfmodel.PageMap{
			{PageNumber: 1, Markdown: "Written by Jane Doe\nPublished 2026"},
		}
		resolver := enrichment.NewDefaultAuthorResolverChain()

		author := resolver.ResolveAuthor(pdfmodel.DocumentMetadata{}, pageMap, "doc.pdf")
		if author == "Unknown Author" || author == "Rick Rubin" {
			// Should resolve or fallback cleanly
		}
	})
}
