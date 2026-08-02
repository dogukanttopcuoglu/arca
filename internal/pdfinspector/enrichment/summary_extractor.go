package enrichment

import (
	"context"
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// SummaryInput holds all semantic artifacts for summary extraction.
type SummaryInput struct {
	Chunks    []pdfmodel.KnowledgeChunk
	Keywords  []pdfmodel.Keyword
	Entities  []pdfmodel.Entity
	Concepts  []pdfmodel.Concept
	Relations []pdfmodel.Relation
}

// SummaryResult contains document-level and chunk-level summaries.
type SummaryResult struct {
	DocumentSummary *pdfmodel.Summary
	ChunkSummaries  map[string]*pdfmodel.Summary
}

// SummaryExtractor defines the strategy seam for producing document and chunk summaries.
type SummaryExtractor interface {
	ExtractSummaries(ctx context.Context, input SummaryInput) (SummaryResult, error)
}

// RuleBasedSummaryExtractor implements extractive summarization by selecting key sentences, concepts, and entities.
type RuleBasedSummaryExtractor struct{}

// NewRuleBasedSummaryExtractor constructs a RuleBasedSummaryExtractor instance.
func NewRuleBasedSummaryExtractor() *RuleBasedSummaryExtractor {
	return &RuleBasedSummaryExtractor{}
}

// ExtractSummaries produces extractive summaries for document and chunks.
func (e *RuleBasedSummaryExtractor) ExtractSummaries(ctx context.Context, input SummaryInput) (SummaryResult, error) {
	if len(input.Chunks) == 0 {
		return SummaryResult{
			ChunkSummaries: make(map[string]*pdfmodel.Summary),
		}, nil
	}

	chunkSummaries := make(map[string]*pdfmodel.Summary)
	var docSentences []string

	for _, ch := range input.Chunks {
		sentences := splitSentences(ch.ContentMarkdown)
		if len(sentences) > 0 {
			firstSentence := strings.TrimSpace(sentences[0])
			if firstSentence != "" {
				chunkSummaries[ch.ChunkID] = &pdfmodel.Summary{
					Text:   firstSentence,
					Source: pdfmodel.SummarySourceRuleBased,
				}
				if len(docSentences) < 3 {
					docSentences = append(docSentences, firstSentence)
				}
			}
		}
	}

	// Synthesize DocumentSummary extractively
	var docText string
	if len(docSentences) > 0 {
		docText = strings.Join(docSentences, " ")
	}

	if len(input.Concepts) > 0 {
		var conceptNames []string
		for i, c := range input.Concepts {
			if i >= 3 {
				break
			}
			conceptNames = append(conceptNames, c.Name)
		}
		if docText != "" {
			docText += " Key Topics: " + strings.Join(conceptNames, ", ") + "."
		} else {
			docText = "Key Topics: " + strings.Join(conceptNames, ", ") + "."
		}
	}

	var docSummary *pdfmodel.Summary
	if docText != "" {
		docSummary = &pdfmodel.Summary{
			Text:   docText,
			Source: pdfmodel.SummarySourceRuleBased,
		}
	}

	return SummaryResult{
		DocumentSummary: docSummary,
		ChunkSummaries:  chunkSummaries,
	}, nil
}

func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\n", " ")
	var sentences []string
	raw := strings.Split(text, ". ")
	for _, s := range raw {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			if !strings.HasSuffix(trimmed, ".") {
				trimmed += "."
			}
			sentences = append(sentences, trimmed)
		}
	}
	return sentences
}
