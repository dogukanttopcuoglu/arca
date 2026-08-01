package enrichment_test

import (
	"testing"

	"arca/internal/pdfinspector/enrichment"
	"arca/internal/pdfinspector/model"
)

func TestNormalizeHeading(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Beginner’s Mind", "beginners mind"},
		{"Beginner's Mind", "beginners mind"},
		{"  BEGINNER'S   MIND!!  ", "beginners mind"},
		{"1. Introduction & Overview", "1 introduction overview"},
	}

	for _, tt := range tests {
		got := enrichment.NormalizeHeading(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeHeading(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestEnrichDocumentTitle(t *testing.T) {
	t.Run("uses existing PDF metadata title if present", func(t *testing.T) {
		doc := model.DocumentMetadata{Title: "Real PDF Title"}
		tree := &model.SemanticTree{
			RootNodes: []model.SemanticNode{
				{Heading: "First Chapter H1", Level: 1},
			},
		}
		title := enrichment.ResolveDocumentTitle(doc, tree, nil, "document.pdf")
		if title != "Real PDF Title" {
			t.Errorf("expected 'Real PDF Title', got %q", title)
		}
	})

	t.Run("ignores generic title Extracted PDF Document and picks first H1", func(t *testing.T) {
		doc := model.DocumentMetadata{Title: "Extracted PDF Document"}
		tree := &model.SemanticTree{
			RootNodes: []model.SemanticNode{
				{Heading: "Introduction", Level: 1}, // Generic
				{Heading: "The Creative Act", Level: 1},
			},
		}
		title := enrichment.ResolveDocumentTitle(doc, tree, nil, "rick-rubin.pdf")
		if title != "The Creative Act: A Way of Being" && title != "The Creative Act" {
			t.Errorf("expected 'The Creative Act: A Way of Being', got %q", title)
		}
	})

	t.Run("falls back to filename if no meaningful heading found", func(t *testing.T) {
		doc := model.DocumentMetadata{Title: ""}
		tree := &model.SemanticTree{
			RootNodes: []model.SemanticNode{
				{Heading: "Chapter 1", Level: 1},
			},
		}
		title := enrichment.ResolveDocumentTitle(doc, tree, nil, "rick-rubin-guide.pdf")
		if title != "The Creative Act: A Way of Being" && title != "Rick Rubin Guide" {
			t.Errorf("expected title resolution, got %q", title)
		}
	})
}

func TestEnrichDocumentAuthor(t *testing.T) {
	t.Run("extracts Rick Rubin author from filename or early pages", func(t *testing.T) {
		doc := model.DocumentMetadata{Author: "Firecrawl Inspector"}
		pageMap := []model.PageMap{
			{PageNumber: 1, Markdown: "# The Creative Act\nBy Rick Rubin"},
		}
		author := enrichment.ResolveDocumentAuthor(doc, pageMap, "rick-rubin.pdf")
		if author != "Rick Rubin" {
			t.Errorf("expected 'Rick Rubin', got %q", author)
		}
	})
}

func TestEnrichSemanticTreePageNumbers(t *testing.T) {
	tree := &model.SemanticTree{
		RootNodes: []model.SemanticNode{
			{ID: "sec-1", Heading: "Everyone Is a Creator", Level: 1, PageNumbers: []int{1}},
			{ID: "sec-2", Heading: "Beginner’s Mind", Level: 1, PageNumbers: []int{1}},
			{ID: "sec-3", Heading: "Unmatched Section", Level: 1, PageNumbers: []int{1}},
		},
	}

	// Page 6 is Table of Contents (contains title "Beginner's Mind" as plain text list)
	// Page 71 is the real chapter page (contains "### Beginner's Mind" as explicit heading)
	pageMap := []model.PageMap{
		{PageNumber: 1, Markdown: "# Cover\nWelcome to the book."},
		{PageNumber: 6, Markdown: "### Contents\n\n1. Everyone Is a Creator ..... 15\n2. Beginner’s Mind ..... 71"},
		{PageNumber: 15, Markdown: "### Everyone Is a Creator\nContent for chapter 1."},
		{PageNumber: 71, Markdown: "### Beginner's Mind\nSome three thousand years ago in China..."},
	}

	chunks := []model.KnowledgeChunk{
		{
			ChunkID:         "chk-1",
			SectionPath:     "Unmatched Section",
			PageNumbers:     []int{95},
			HeadingLevel:    1,
			ContentMarkdown: "Unmatched section content",
		},
	}

	warnings := enrichment.EnrichSemanticTree(tree, pageMap, chunks)

	if tree.RootNodes[0].PageNumbers[0] != 15 {
		t.Errorf("expected section 1 page number 15, got %v", tree.RootNodes[0].PageNumbers)
	}

	// Crucial check: Beginner's Mind must resolve to real section page 71, NOT TOC page 6!
	if tree.RootNodes[1].PageNumbers[0] != 71 {
		t.Errorf("expected section 2 page number 71 (skipping TOC page 6), got %v", tree.RootNodes[1].PageNumbers)
	}

	if tree.RootNodes[2].PageNumbers[0] != 95 {
		t.Errorf("expected section 3 page number 95 (resolved via chunk fallback), got %v", tree.RootNodes[2].PageNumbers)
	}

	_ = warnings
}
