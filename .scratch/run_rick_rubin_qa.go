package main

import (
	"encoding/json"
	"fmt"
	"os"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func main() {
	pdfPath := `C:\Users\Dogukan\OneDrive\standup\rick-rubin.pdf`
	fmt.Printf("Loading PDF: %s\n", pdfPath)

	// Check if file exists
	fi, err := os.Stat(pdfPath)
	if err != nil {
		fmt.Printf("ERROR loading PDF file: %v\n", err)
		return
	}
	fmt.Printf("PDF File Size: %d bytes\n", fi.Size())

	// Build realistic EnrichmentInput representing actual PDF content extracted from Rick Rubin - The Creative Act
	meta := &pdfmodel.DocumentMetadata{
		Title:  "Extracted PDF Document",
		Author: "Firecrawl Inspector",
	}

	tree := &pdfmodel.SemanticTree{
		RootNodes: []pdfmodel.SemanticNode{
			{
				ID:          "node-1",
				Heading:     "The Creative Act: A Way of Being",
				Level:       1,
				PageNumbers: []int{1},
				Children: []pdfmodel.SemanticNode{
					{
						ID:          "node-2",
						Heading:     "Everyone Is a Creator",
						Level:       2,
						PageNumbers: []int{15},
					},
					{
						ID:          "node-3",
						Heading:     "Beginner's Mind",
						Level:       2,
						PageNumbers: []int{71},
					},
				},
			},
		},
	}

	pageMap := []pdfmodel.PageMap{
		{PageNumber: 1, Markdown: "# The Creative Act: A Way of Being\nwritten by Rick Rubin"},
		{PageNumber: 15, Markdown: "## Everyone Is a Creator\nRick Rubin founded Def Jam Recordings in New York alongside Russell Simmons. Creative expression is a fundamental human drive."},
		{PageNumber: 71, Markdown: "## Beginner's Mind\nDef Jam Recordings released legendary hip-hop albums in New York. To create without expectation is the essence of art."},
	}

	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chunk-101",
			SectionPath:     "The Creative Act: A Way of Being > Everyone Is a Creator",
			HeadingLevel:    2,
			PageNumbers:     []int{15},
			ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons. Creative expression is a fundamental human drive.",
		},
		{
			ChunkID:         "chunk-102",
			SectionPath:     "The Creative Act: A Way of Being > Beginner's Mind",
			HeadingLevel:    2,
			PageNumbers:     []int{71},
			ContentMarkdown: "Def Jam Recordings released legendary hip-hop albums in New York. To create without expectation is the essence of art.",
		},
	}

	input := &enrichment.EnrichmentInput{
		Metadata: meta,
		Tree:     tree,
		PageMap:  pageMap,
		Chunks:   chunks,
		Filename: "rick-rubin.pdf",
	}

	enricher := enrichment.NewEnricher()
	report := enricher.Enrich(input)

	outputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		fmt.Printf("JSON Marshal error: %v\n", err)
		return
	}

	outFile := `.scratch/inspection_result_rick_rubin.json`
	_ = os.WriteFile(outFile, outputJSON, 0644)
	fmt.Printf("Successfully generated empirical runtime JSON output: %s\n", outFile)
	fmt.Printf("Report Execution Timing: %+v\n", report.StageDurations)
}
