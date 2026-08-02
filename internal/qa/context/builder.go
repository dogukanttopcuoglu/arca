package context

import (
	"context"
	"fmt"
	"strings"

	indexingmodel "arca/internal/indexing/model"
	retrievalseam "arca/internal/retrieval/seam"
)

// SourceReference represents an immutable source document chunk reference bound to a prompt citation key (`[Ref N]`).
type SourceReference struct {
	CitationKey string                       `json:"citation_key"`
	DocumentID  string                       `json:"document_id"`
	ChunkID     string                       `json:"chunk_id"`
	SectionPath string                       `json:"section_path,omitempty"`
	PageNumbers []int                        `json:"page_numbers,omitempty"`
	Content     string                       `json:"content"`
	Metadata    indexingmodel.VectorMetadata `json:"metadata"`
}

// ContextWindow encapsulates the prompt-ready formatted text, source reference mapping, and total token count.
type ContextWindow struct {
	Sources    []SourceReference `json:"sources"`
	Content    string            `json:"content"`
	TokenCount int               `json:"token_count"`
}

// ContextBuilder defines the seam for building prompt-ready ContextWindow structures from retrieval search results.
type ContextBuilder interface {
	Build(ctx context.Context, results []retrievalseam.SearchResult) (*ContextWindow, error)
}

// DefaultContextBuilder implements ContextBuilder with deduplication, token budgeting, and citation key assignment.
type DefaultContextBuilder struct {
	tokenCounter TokenCounter
	maxTokens    int
}

// NewDefaultContextBuilder constructs a DefaultContextBuilder instance.
func NewDefaultContextBuilder(counter TokenCounter, maxTokens int) *DefaultContextBuilder {
	if counter == nil {
		counter = NewSimpleTokenCounter()
	}
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	return &DefaultContextBuilder{
		tokenCounter: counter,
		maxTokens:    maxTokens,
	}
}

// Build processes search results into a deduplicated, budget-limited ContextWindow.
func (b *DefaultContextBuilder) Build(ctx context.Context, results []retrievalseam.SearchResult) (*ContextWindow, error) {
	seenChunks := make(map[string]bool)
	var sources []SourceReference

	totalTokens := 0
	var sb strings.Builder

	refIndex := 1
	for _, res := range results {
		if res.ChunkID == "" || seenChunks[res.ChunkID] {
			continue
		}

		seenChunks[res.ChunkID] = true
		citationKey := fmt.Sprintf("[Ref %d]", refIndex)

		sourceText := res.ContentMarkdown
		if sourceText == "" {
			sourceText = fmt.Sprintf("Document %s, Section %s", res.Metadata.DocumentID, res.Metadata.SectionPath)
		}

		sourceBlock := fmt.Sprintf("%s Source: %s (Page %v, Section: %s)\n%s\n\n",
			citationKey, res.Metadata.DocumentID, res.Metadata.PageNumbers, res.Metadata.SectionPath, sourceText)

		blockTokens := b.tokenCounter.Count(sourceBlock)
		if len(sources) > 0 && totalTokens+blockTokens > b.maxTokens {
			// Budget exceeded, stop accumulating further sources
			break
		}

		sb.WriteString(sourceBlock)
		totalTokens += blockTokens

		sources = append(sources, SourceReference{
			CitationKey: citationKey,
			DocumentID:  res.Metadata.DocumentID,
			ChunkID:     res.ChunkID,
			SectionPath: res.Metadata.SectionPath,
			PageNumbers: res.Metadata.PageNumbers,
			Content:     sourceText,
			Metadata:    res.Metadata,
		})

		refIndex++
	}

	return &ContextWindow{
		Sources:    sources,
		Content:    sb.String(),
		TokenCount: totalTokens,
	}, nil
}
