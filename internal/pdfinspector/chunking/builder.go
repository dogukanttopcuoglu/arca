package chunking

import (
	"context"
	"fmt"
	"strings"

	"arca/internal/pdfinspector/model"
)

// ChunkBuilder consumes intermediate SemanticBlock structures and constructs deterministic KnowledgeChunk objects.
type ChunkBuilder interface {
	Build(ctx context.Context, blocks []SemanticBlock, opts Options, collector *WarningCollector) ([]model.KnowledgeChunk, error)
}

// DefaultChunkBuilder implements ChunkBuilder.
type DefaultChunkBuilder struct{}

// NewChunkBuilder creates a DefaultChunkBuilder instance.
func NewChunkBuilder() *DefaultChunkBuilder {
	return &DefaultChunkBuilder{}
}

// Build transforms semantic blocks into structured KnowledgeChunk items with parent-child links, neighbor links, and hashes.
func (b *DefaultChunkBuilder) Build(ctx context.Context, blocks []SemanticBlock, opts Options, collector *WarningCollector) ([]model.KnowledgeChunk, error) {
	if len(blocks) == 0 {
		return []model.KnowledgeChunk{}, nil
	}

	sizer := opts.Sizer
	if sizer == nil {
		sizer = NewHeuristicSizer()
	}

	// Group blocks into raw chunk candidates based on section path and token bounds
	type blockGroup struct {
		sectionPath      string
		headingLevel     int
		blocks           []SemanticBlock
		kind             BlockKind
		category         model.SemanticCategory
		estimatedTokens  int
		isOversized      bool
	}

	var groups []blockGroup
	var currentGroup *blockGroup

	flushGroup := func() {
		if currentGroup != nil && len(currentGroup.blocks) > 0 {
			groups = append(groups, *currentGroup)
			currentGroup = nil
		}
	}

	for _, block := range blocks {
		tokens := sizer.Size(block.Markdown)

		// Check if single atomic block is oversized (> AbsoluteMaxTokens)
		isBlockOversized := tokens > opts.AbsoluteMaxTokens
		if isBlockOversized && collector != nil {
			collector.AddWarning(fmt.Sprintf("oversized chunk encountered in section %q: token estimate %d exceeds absolute max limit %d", block.SectionPath, tokens, opts.AbsoluteMaxTokens))
		}

		// Non-paragraph atomic blocks (table, code, equation, figure, list) stay intact
		isAtomic := block.Kind == KindTable || block.Kind == KindCode || block.Kind == KindEquation || block.Kind == KindFigure || block.Kind == KindList

		if isAtomic || isBlockOversized {
			flushGroup()
			groups = append(groups, blockGroup{
				sectionPath:     block.SectionPath,
				headingLevel:    block.HeadingLevel,
				blocks:          []SemanticBlock{block},
				kind:            block.Kind,
				category:        block.SemanticCategory,
				estimatedTokens: tokens,
				isOversized:     isBlockOversized,
			})
			continue
		}

		// Handle grouping paragraphs and lists within section path up to TargetMaxTokens
		if currentGroup == nil {
			currentGroup = &blockGroup{
				sectionPath:     block.SectionPath,
				headingLevel:    block.HeadingLevel,
				blocks:          []SemanticBlock{block},
				kind:            block.Kind,
				category:        block.SemanticCategory,
				estimatedTokens: tokens,
			}
		} else if currentGroup.sectionPath == block.SectionPath && (currentGroup.estimatedTokens+tokens <= opts.TargetMaxTokens) {
			currentGroup.blocks = append(currentGroup.blocks, block)
			currentGroup.estimatedTokens += tokens
		} else {
			flushGroup()
			currentGroup = &blockGroup{
				sectionPath:     block.SectionPath,
				headingLevel:    block.HeadingLevel,
				blocks:          []SemanticBlock{block},
				kind:            block.Kind,
				category:        block.SemanticCategory,
				estimatedTokens: tokens,
			}
		}
	}
	flushGroup()

	// Organize groups by section to construct parent-child hierarchy when section has multiple child chunks
	type sectionChunks struct {
		sectionPath  string
		headingLevel int
		childIndices []int
	}

	sectionMap := make(map[string]*sectionChunks)
	var sectionOrder []string

	for i, g := range groups {
		sc, exists := sectionMap[g.sectionPath]
		if !exists {
			sc = &sectionChunks{
				sectionPath:  g.sectionPath,
				headingLevel: g.headingLevel,
				childIndices: []int{},
			}
			sectionMap[g.sectionPath] = sc
			sectionOrder = append(sectionOrder, g.sectionPath)
		}
		sc.childIndices = append(sc.childIndices, i)
	}

	var resultChunks []model.KnowledgeChunk
	sectionSlugOrdinal := make(map[string]int)

	// Build chunks
	for _, secPath := range sectionOrder {
		sc := sectionMap[secPath]
		slug := Slugify(secPath)

		// Create section parent chunk if section has >1 child chunks or parent context
		var parentChunkID *string
		if len(sc.childIndices) > 1 {
			sectionSlugOrdinal[slug]++
			pID := fmt.Sprintf("%s/%s/%03d", opts.DocumentID, slug, sectionSlugOrdinal[slug])
			parentChunkID = &pID

			// Collect combined text & page range for parent chunk
			var parentMarkdownParts []string
			pageSet := make(map[int]bool)
			var parentCitations []model.Citation
			startChar := -1
			endChar := 0

			childIDs := make([]string, 0, len(sc.childIndices))
			for cIdx, gIdx := range sc.childIndices {
				sectionSlugOrdinal[slug]++
				cID := fmt.Sprintf("%s/%s/%03d", opts.DocumentID, slug, sectionSlugOrdinal[slug])
				childIDs = append(childIDs, cID)

				g := groups[gIdx]
				for _, b := range g.blocks {
					parentMarkdownParts = append(parentMarkdownParts, b.Markdown)
					for _, pg := range b.PageNumbers {
						pageSet[pg] = true
					}
					parentCitations = append(parentCitations, b.Citations...)
					if startChar == -1 || b.SourceOffsets.StartChar < startChar {
						startChar = b.SourceOffsets.StartChar
					}
					if b.SourceOffsets.EndChar > endChar {
						endChar = b.SourceOffsets.EndChar
					}
				}

				_ = cIdx
			}

			pages := sortedPageNumbers(pageSet)
			parentMarkdown := strings.Join(parentMarkdownParts, "\n\n")
			pTokens := sizer.Size(parentMarkdown)

			// Emit parent chunk
			parentChunk := model.KnowledgeChunk{
				ChunkID:          *parentChunkID,
				ParentChunkID:    nil,
				ChildChunkIDs:    childIDs,
				DocumentID:       opts.DocumentID,
				SectionPath:      sc.sectionPath,
				HeadingLevel:     sc.headingLevel,
				PageNumbers:      pages,
				ContentMarkdown:  parentMarkdown,
				TokenEstimate:    pTokens,
				CharacterCount:   len(parentMarkdown),
				Citations:        dedupeCitations(parentCitations),
				SourceOffsets:    model.SourceOffset{StartChar: startChar, EndChar: endChar},
				ContentType:      model.ContentTypeParagraph,
				SemanticCategory: model.SemanticNarrative,
				ContentHash:      ComputeContentHash(parentMarkdown),
				Fingerprint:      ComputeFingerprint(opts.DocumentID, sc.sectionPath, sc.headingLevel, parentMarkdown, pages),
				IsOversized:      pTokens > opts.AbsoluteMaxTokens,
			}
			resultChunks = append(resultChunks, parentChunk)

			// Emit child chunks matching childIDs
			for cIdx, gIdx := range sc.childIndices {
				cID := childIDs[cIdx]
				g := groups[gIdx]

				var childMarkdownParts []string
				cPageSet := make(map[int]bool)
				var childCitations []model.Citation
				cStartChar := -1
				cEndChar := 0

				for _, b := range g.blocks {
					childMarkdownParts = append(childMarkdownParts, b.Markdown)
					for _, pg := range b.PageNumbers {
						cPageSet[pg] = true
					}
					childCitations = append(childCitations, b.Citations...)
					if cStartChar == -1 || b.SourceOffsets.StartChar < cStartChar {
						cStartChar = b.SourceOffsets.StartChar
					}
					if b.SourceOffsets.EndChar > cEndChar {
						cEndChar = b.SourceOffsets.EndChar
					}
				}

				cPages := sortedPageNumbers(cPageSet)
				childMarkdown := strings.Join(childMarkdownParts, "\n\n")
				cTokens := g.estimatedTokens

				childChunk := model.KnowledgeChunk{
					ChunkID:          cID,
					ParentChunkID:    parentChunkID,
					ChildChunkIDs:    []string{},
					DocumentID:       opts.DocumentID,
					SectionPath:      sc.sectionPath,
					HeadingLevel:     sc.headingLevel,
					PageNumbers:      cPages,
					ContentMarkdown:  childMarkdown,
					TokenEstimate:    cTokens,
					CharacterCount:   len(childMarkdown),
					Citations:        dedupeCitations(childCitations),
					SourceOffsets:    model.SourceOffset{StartChar: cStartChar, EndChar: cEndChar},
					ContentType:      string(g.kind),
					SemanticCategory: g.category,
					ContentHash:      ComputeContentHash(childMarkdown),
					Fingerprint:      ComputeFingerprint(opts.DocumentID, sc.sectionPath, sc.headingLevel, childMarkdown, cPages),
					IsOversized:      g.isOversized,
				}
				resultChunks = append(resultChunks, childChunk)
			}
		} else {
			// Single chunk for section
			gIdx := sc.childIndices[0]
			g := groups[gIdx]

			sectionSlugOrdinal[slug]++
			cID := fmt.Sprintf("%s/%s/%03d", opts.DocumentID, slug, sectionSlugOrdinal[slug])

			var markdownParts []string
			pageSet := make(map[int]bool)
			var cits []model.Citation
			startChar := -1
			endChar := 0

			for _, b := range g.blocks {
				markdownParts = append(markdownParts, b.Markdown)
				for _, pg := range b.PageNumbers {
					pageSet[pg] = true
				}
				cits = append(cits, b.Citations...)
				if startChar == -1 || b.SourceOffsets.StartChar < startChar {
					startChar = b.SourceOffsets.StartChar
				}
				if b.SourceOffsets.EndChar > endChar {
					endChar = b.SourceOffsets.EndChar
				}
			}

			pages := sortedPageNumbers(pageSet)
			markdown := strings.Join(markdownParts, "\n\n")

			chunk := model.KnowledgeChunk{
				ChunkID:          cID,
				ParentChunkID:    nil,
				ChildChunkIDs:    []string{},
				DocumentID:       opts.DocumentID,
				SectionPath:      sc.sectionPath,
				HeadingLevel:     sc.headingLevel,
				PageNumbers:      pages,
				ContentMarkdown:  markdown,
				TokenEstimate:    g.estimatedTokens,
				CharacterCount:   len(markdown),
				Citations:        dedupeCitations(cits),
				SourceOffsets:    model.SourceOffset{StartChar: startChar, EndChar: endChar},
				ContentType:      string(g.kind),
				SemanticCategory: g.category,
				ContentHash:      ComputeContentHash(markdown),
				Fingerprint:      ComputeFingerprint(opts.DocumentID, sc.sectionPath, sc.headingLevel, markdown, pages),
				IsOversized:      g.isOversized,
			}
			resultChunks = append(resultChunks, chunk)
		}
	}

	// Link sequential neighbors (PreviousChunkID, NextChunkID, ChunkOrder)
	for i := range resultChunks {
		resultChunks[i].ChunkOrder = i + 1
		if i > 0 {
			prevID := resultChunks[i-1].ChunkID
			resultChunks[i].PreviousChunkID = &prevID
		}
		if i < len(resultChunks)-1 {
			nextID := resultChunks[i+1].ChunkID
			resultChunks[i].NextChunkID = &nextID
		}
	}

	return resultChunks, nil
}

func sortedPageNumbers(pageSet map[int]bool) []int {
	if len(pageSet) == 0 {
		return []int{1}
	}
	pages := make([]int, 0, len(pageSet))
	for pg := range pageSet {
		if pg >= 1 {
			pages = append(pages, pg)
		}
	}
	if len(pages) == 0 {
		return []int{1}
	}
	// Bubble sort small slice
	for i := 0; i < len(pages)-1; i++ {
		for j := i + 1; j < len(pages); j++ {
			if pages[i] > pages[j] {
				pages[i], pages[j] = pages[j], pages[i]
			}
		}
	}
	return pages
}

func dedupeCitations(cits []model.Citation) []model.Citation {
	if len(cits) == 0 {
		return []model.Citation{}
	}
	res := make([]model.Citation, 0, len(cits))
	seen := make(map[string]bool)
	for _, c := range cits {
		if !seen[c.RawText] {
			seen[c.RawText] = true
			res = append(res, c)
		}
	}
	return res
}
