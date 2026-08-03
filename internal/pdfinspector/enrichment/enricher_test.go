package enrichment_test

import (
	"context"
	"strings"
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

func TestDefaultEnricher(t *testing.T) {
	enricher := enrichment.NewEnricher()

	meta := &model.DocumentMetadata{
		Title:  "Extracted PDF Document",
		Author: "Firecrawl Inspector",
	}
	tree := &model.SemanticTree{
		RootNodes: []model.SemanticNode{
			{Heading: "Beginner's Mind", Level: 1, PageNumbers: []int{1}},
		},
	}
	pageMap := []model.PageMap{
		{PageNumber: 1, Markdown: "# Cover\nwritten by Rick Rubin"},
		{PageNumber: 71, Markdown: "### Beginner's Mind\nSome content"},
	}

	report := enricher.Enrich(context.Background(), &enrichment.EnrichmentInput{
		Metadata: meta,
		Tree:     tree,
		PageMap:  pageMap,
		Filename: "rick-rubin.pdf",
	})

	if meta.Title == "" {
		t.Errorf("expected non-empty resolved Title, got %q", meta.Title)
	}
	if tree.RootNodes[0].PageNumbers[0] != 71 {
		t.Errorf("expected page 71, got %v", tree.RootNodes[0].PageNumbers)
	}
	if report == nil {
		t.Fatal("expected non-nil EnrichmentReport")
	}
}

func TestDefaultEnricherPropagatesPageResolutionWarnings(t *testing.T) {
	enricher := enrichment.NewEnricher()

	meta := &model.DocumentMetadata{
		Title:  "Some Title",
		Author: "Some Author",
	}
	tree := &model.SemanticTree{
		RootNodes: []model.SemanticNode{
			{Heading: "Unmatched Section", Level: 1, PageNumbers: []int{1}},
		},
	}
	pageMap := []model.PageMap{
		{PageNumber: 1, Markdown: "# Cover\nwritten by Rick Rubin"},
	}

	report := enricher.Enrich(context.Background(), &enrichment.EnrichmentInput{
		Metadata: meta,
		Tree:     tree,
		PageMap:  pageMap,
		Filename: "rick-rubin.pdf",
	})
	if report == nil {
		t.Fatal("expected non-nil EnrichmentReport")
	}

	found := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "semantic page resolution unavailable") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected page-resolution warning surfaced in report.Warnings, got %v", report.Warnings)
	}
}

func TestDefaultEnricherWiresMetadataConsistencyPass(t *testing.T) {
	enricher := enrichment.NewEnricher()

	meta := &model.DocumentMetadata{
		Title:      "Some Title",
		Author:     "Some Author",
		PageCount:  0,
		Searchable: false,
	}
	pageMap := []model.PageMap{
		{PageNumber: 1, Markdown: "# Cover\nwritten by Rick Rubin"},
		{PageNumber: 2, Markdown: "Some readable text"},
	}
	chunks := []model.KnowledgeChunk{
		{ChunkID: "chk-1", SectionPath: "S", PageNumbers: []int{3}, ContentMarkdown: "content"},
	}

	report := enricher.Enrich(context.Background(), &enrichment.EnrichmentInput{
		Metadata: meta,
		Tree:     &model.SemanticTree{},
		PageMap:  pageMap,
		Chunks:   chunks,
		Filename: "rick-rubin.pdf",
	})
	if report == nil {
		t.Fatal("expected non-nil EnrichmentReport")
	}

	if meta.PageCount != 3 {
		t.Errorf("expected PageCount resolved to 3 (highest chunk page), got %d", meta.PageCount)
	}
	if !meta.Searchable {
		t.Error("expected Searchable resolved to true from non-empty PageMap")
	}
}
