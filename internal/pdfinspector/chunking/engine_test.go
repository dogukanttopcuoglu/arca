package chunking_test

import (
	"context"
	"strings"
	"testing"

	"arca/internal/pdfinspector/chunking"
	"arca/internal/pdfinspector/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkDocument_BasicParagraphs(t *testing.T) {
	eng := chunking.NewEngine(chunking.WithDocumentID("doc-test-101"))
	md := `# Introduction
This is the intro paragraph for ARC architecture.

## Background
Detailed background information about knowledge parsing.`

	tree := &model.SemanticTree{
		RootNodes: []model.SemanticNode{
			{
				ID:          "sec-1",
				Heading:     "Introduction",
				Level:       1,
				PageNumbers: []int{1},
				Children: []model.SemanticNode{
					{
						ID:          "sec-1-1",
						Heading:     "Background",
						Level:       2,
						PageNumbers: []int{1},
					},
				},
			},
		},
	}

	chunks, err := eng.ChunkDocument(context.Background(), "doc-test-101", tree, md, nil)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	// Build PDFInspectionResult and validate whole model
	res := model.NewPDFInspectionResult()
	res.Document.Title = "Test Doc"
	res.Document.PageCount = 1
	res.SemanticTree = *tree
	res.Chunks = chunks
	require.NoError(t, res.Validate())

	for i, chunk := range chunks {
		assert.NoError(t, chunk.Validate())
		assert.Equal(t, "doc-test-101", chunk.DocumentID)
		assert.Equal(t, i+1, chunk.ChunkOrder)
		assert.NotEmpty(t, chunk.ContentHash)
		assert.NotEmpty(t, chunk.Fingerprint)
		assert.True(t, strings.HasPrefix(chunk.ChunkID, "doc-test-101/"))
	}
}

func TestChunkDocument_BoundaryPreservation(t *testing.T) {
	eng := chunking.NewEngine(chunking.WithDocumentID("doc-boundary-1"))
	md := "# Technical Overview\n\nBelow is sample code:\n\n```go\nfunc main() {\n\tfmt.Println(\"Hello ARC\")\n}\n```\n\nBelow is a table:\n\n| Parameter | Type | Description |\n|-----------|------|-------------|\n| Timeout   | int  | Milliseconds|\n\nBelow is an equation:\n\n$$\nE = mc^2\n$$\n\nBelow is a list:\n\n- Step 1: Parse\n- Step 2: Chunk\n- Step 3: Index"

	chunks, err := eng.ChunkDocument(context.Background(), "doc-boundary-1", nil, md, nil)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	hasCode := false
	hasTable := false
	hasEquation := false
	hasList := false

	for _, c := range chunks {
		require.NoError(t, c.Validate())
		switch c.ContentType {
		case model.ContentTypeCode:
			hasCode = true
			assert.Equal(t, model.SemanticCode, c.SemanticCategory)
			assert.Contains(t, c.ContentMarkdown, "func main()")
		case model.ContentTypeTable:
			hasTable = true
			assert.Equal(t, model.SemanticTable, c.SemanticCategory)
			assert.Contains(t, c.ContentMarkdown, "| Timeout   | int  |")
		case model.ContentTypeEquation:
			hasEquation = true
			assert.Equal(t, model.SemanticEquation, c.SemanticCategory)
			assert.Contains(t, c.ContentMarkdown, "E = mc^2")
		case model.ContentTypeList:
			hasList = true
			assert.Equal(t, model.SemanticProcedure, c.SemanticCategory)
			assert.Contains(t, c.ContentMarkdown, "- Step 1: Parse")
		}
	}

	assert.True(t, hasCode, "expected code chunk")
	assert.True(t, hasTable, "expected table chunk")
	assert.True(t, hasEquation, "expected equation chunk")
	assert.True(t, hasList, "expected list chunk")
}

func TestChunkDocument_ParentChildHierarchy(t *testing.T) {
	// Set small target max bounds so section splits into multiple child chunks
	eng := chunking.NewEngine(
		chunking.WithDocumentID("doc-parent-child"),
		chunking.WithTargetBounds(10, 20),
	)

	md := `# Section One
First block of content for section one with some extra words to fill token limit.

Second block of content for section one with additional details to force chunking split.

Third block of content for section one to ensure parent chunk is constructed.`

	chunks, err := eng.ChunkDocument(context.Background(), "doc-parent-child", nil, md, nil)
	require.NoError(t, err)

	res := model.NewPDFInspectionResult()
	res.Chunks = chunks
	require.NoError(t, res.Validate())

	var parentChunk *model.KnowledgeChunk
	for i := range chunks {
		if chunks[i].ParentChunkID == nil && len(chunks[i].ChildChunkIDs) > 0 {
			parentChunk = &chunks[i]
			break
		}
	}

	require.NotNil(t, parentChunk, "expected parent chunk for multi-chunk section")
	assert.Len(t, parentChunk.ChildChunkIDs, 3)

	for _, childID := range parentChunk.ChildChunkIDs {
		foundChild := false
		for _, c := range chunks {
			if c.ChunkID == childID {
				foundChild = true
				require.NotNil(t, c.ParentChunkID)
				assert.Equal(t, parentChunk.ChunkID, *c.ParentChunkID)
			}
		}
		assert.True(t, foundChild, "child ID %s should exist in returned chunks", childID)
	}
}

func TestChunkDocument_SequentialNeighbors(t *testing.T) {
	eng := chunking.NewEngine(chunking.WithDocumentID("doc-seq"))
	md := `# Section 1
Paragraph 1.

# Section 2
Paragraph 2.

# Section 3
Paragraph 3.`

	chunks, err := eng.ChunkDocument(context.Background(), "doc-seq", nil, md, nil)
	require.NoError(t, err)
	require.Len(t, chunks, 3)

	assert.Nil(t, chunks[0].PreviousChunkID)
	require.NotNil(t, chunks[0].NextChunkID)
	assert.Equal(t, chunks[1].ChunkID, *chunks[0].NextChunkID)

	require.NotNil(t, chunks[1].PreviousChunkID)
	assert.Equal(t, chunks[0].ChunkID, *chunks[1].PreviousChunkID)
	require.NotNil(t, chunks[1].NextChunkID)
	assert.Equal(t, chunks[2].ChunkID, *chunks[1].NextChunkID)

	require.NotNil(t, chunks[2].PreviousChunkID)
	assert.Equal(t, chunks[1].ChunkID, *chunks[2].PreviousChunkID)
	assert.Nil(t, chunks[2].NextChunkID)
}

func TestChunkDocument_OversizedElementWarning(t *testing.T) {
	// Set absolute max to 15 tokens
	eng := chunking.NewEngine(
		chunking.WithDocumentID("doc-oversized"),
		chunking.WithMaxBounds(10, 15),
	)

	longCode := "```go\n" + strings.Repeat("fmt.Println(\"very long line of code\"); ", 20) + "\n```"
	md := "# Code Section\n\n" + longCode

	chunks, err := eng.ChunkDocument(context.Background(), "doc-oversized", nil, md, nil)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	warnings := eng.Warnings()
	require.NotEmpty(t, warnings, "expected diagnostic warning for oversized chunk")

	hasOversizedWarning := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "oversized") {
			hasOversizedWarning = true
		}
	}
	assert.True(t, hasOversizedWarning, "warnings should mention oversized chunk")

	codeChunkFound := false
	for _, c := range chunks {
		if c.ContentType == model.ContentTypeCode {
			codeChunkFound = true
			assert.True(t, c.IsOversized, "code chunk should be marked as oversized")
		}
	}
	assert.True(t, codeChunkFound, "code chunk should exist and retain boundary")
}

func TestChunkDocument_RequiresDocumentID(t *testing.T) {
	eng := chunking.NewEngine()
	md := "# Section\n\nSome content."
	tree := &model.SemanticTree{
		RootNodes: []model.SemanticNode{
			{ID: "sec-1", Heading: "Section", Level: 1, PageNumbers: []int{1}},
		},
	}

	t.Run("empty document id fails loudly instead of defaulting", func(t *testing.T) {
		_, err := eng.ChunkDocument(context.Background(), "", tree, md, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "document id")
	})

	t.Run("per-call document id overrides construction-time default", func(t *testing.T) {
		engWithDefault := chunking.NewEngine(chunking.WithDocumentID("doc-from-option"))

		chunks, err := engWithDefault.ChunkDocument(context.Background(), "doc-per-call", tree, md, nil)
		require.NoError(t, err)
		require.NotEmpty(t, chunks)
		assert.Equal(t, "doc-per-call", chunks[0].DocumentID)
		assert.True(t, strings.HasPrefix(chunks[0].ChunkID, "doc-per-call/"))
	})

	t.Run("construction-time default used when per-call id empty", func(t *testing.T) {
		engWithDefault := chunking.NewEngine(chunking.WithDocumentID("doc-from-option"))

		chunks, err := engWithDefault.ChunkDocument(context.Background(), "", tree, md, nil)
		require.NoError(t, err)
		require.NotEmpty(t, chunks)
		assert.Equal(t, "doc-from-option", chunks[0].DocumentID)
	})
}

func TestSlugifyAndHash(t *testing.T) {
	slug := chunking.Slugify("Introduction to ARC > Background Details (2026)")
	assert.Equal(t, "introduction-to-arc-background-details-2026", slug)

	h1 := chunking.ComputeContentHash("  Text with trailing space   \n\n")
	h2 := chunking.ComputeContentHash("Text with trailing space")
	assert.Equal(t, h1, h2, "content hash should normalize trailing spaces and blank lines")

	fp1 := chunking.ComputeFingerprint("doc-1", "Intro", 1, "Text", []int{1})
	fp2 := chunking.ComputeFingerprint("doc-1", "Intro", 1, "Text", []int{1})
	fp3 := chunking.ComputeFingerprint("doc-2", "Intro", 1, "Text", []int{1})
	assert.Equal(t, fp1, fp2)
	assert.NotEqual(t, fp1, fp3)
}

func TestChunkDocument_PageTrackingAndCitations(t *testing.T) {
	eng := chunking.NewEngine(chunking.WithDocumentID("doc-pages"))
	md := `# Page One Section
First page paragraph [1].

<!-- page: 2 -->

# Page Two Section
Second page paragraph referencing [Smith et al., 2020].`

	chunks, err := eng.ChunkDocument(context.Background(), "doc-pages", nil, md, nil)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)

	var p1Chunk, p2Chunk *model.KnowledgeChunk
	for i := range chunks {
		if len(chunks[i].PageNumbers) > 0 {
			if chunks[i].PageNumbers[0] == 1 {
				p1Chunk = &chunks[i]
			} else if chunks[i].PageNumbers[0] == 2 {
				p2Chunk = &chunks[i]
			}
		}
	}

	require.NotNil(t, p1Chunk)
	require.NotEmpty(t, p1Chunk.Citations)
	assert.Equal(t, "[1]", p1Chunk.Citations[0].RawText)

	require.NotNil(t, p2Chunk)
	require.NotEmpty(t, p2Chunk.Citations)
	assert.Equal(t, "[Smith et al., 2020]", p2Chunk.Citations[0].RawText)
}

func TestChunkDocument_PageMapResolvesChunkPages(t *testing.T) {
	eng := chunking.NewEngine()
	// The bundled service emits no page markers; json_layout.pages is authoritative.
	md := `# Chapter One
First chapter content paragraph.

# Chapter Two
Second chapter content paragraph.`

	// Page 3 holds Chapter One, page 7 holds Chapter Two (pages in between have no headings).
	pageMap := []model.PageMap{
		{PageNumber: 1, Markdown: "Cover"},
		{PageNumber: 2, Markdown: "Table of contents"},
		{PageNumber: 3, Markdown: "# Chapter One\nFirst chapter content paragraph."},
		{PageNumber: 7, Markdown: "# Chapter Two\nSecond chapter content paragraph."},
	}

	chunks, err := eng.ChunkDocument(context.Background(), "doc-pages-map", nil, md, pageMap)
	require.NoError(t, err)
	require.Len(t, chunks, 2)

	assert.Equal(t, 3, chunks[0].PageNumbers[0], "chapter one should resolve to page 3 from PageMap")
	assert.Equal(t, 7, chunks[1].PageNumbers[0], "chapter two should resolve to page 7 from PageMap")
}

func TestChunkDocument_PageMapResolvesCitationPages(t *testing.T) {
	eng := chunking.NewEngine()
	md := `# Report
Findings described by Smith et al., 2020 [1] on this page.`

	pageMap := []model.PageMap{
		{PageNumber: 1, Markdown: "# Report\nFindings described by Smith et al., 2020 [1] on this page."},
		{PageNumber: 2, Markdown: "Appendix"},
	}

	chunks, err := eng.ChunkDocument(context.Background(), "doc-cit-pages", nil, md, pageMap)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.NotEmpty(t, chunks[0].Citations)
	assert.Equal(t, 1, chunks[0].Citations[0].PageNumber, "citation should carry the resolved page from PageMap")
}
