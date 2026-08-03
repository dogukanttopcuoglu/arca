package enrichment

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"arca/internal/pdfinspector/model"
)

var genericHeadings = map[string]bool{
	"extracted pdf document":                           true,
	"chapter 1":                                        true,
	"introduction":                                     true,
	"contents":                                         true,
	"table of contents":                                true,
	"untitled document":                                true,
	"whats next on your reading list":                  true,
	"whats next on your reading list u discover":       true,
	"whats next on your reading list u discover your":  true,
	"discover your next great read":                    true,
	"about the author":                                 true,
	"copyright":                                        true,
	"all rights reserved":                              true,
	"firecrawl inspector":                              true,
	"sample extracted document":                        true,
}

// EnrichmentInput holds the targeted structural inputs required for semantic metadata enrichment.
type EnrichmentInput struct {
	Metadata *model.DocumentMetadata
	Tree     *model.SemanticTree
	PageMap  []model.PageMap
	Chunks   []model.KnowledgeChunk
	Filename string
}

// EnrichmentReport contains the comprehensive metrics and outcome of the enrichment pass pipeline.
type EnrichmentReport struct {
	Warnings          []string          `json:"warnings"`
	StageDurations    map[string]int64  `json:"stage_durations_ms"`
	ResolvedResolvers map[string]string `json:"resolved_resolvers,omitempty"`
	SkippedStages     []string          `json:"skipped_stages,omitempty"`
}

// Enricher defines the seam for post-extraction semantic metadata enrichment.
// The provided context is propagated to every EnricherPass in the pipeline.
type Enricher interface {
	Enrich(ctx context.Context, input *EnrichmentInput) *EnrichmentReport
}

// DefaultEnricher is the default implementation of the Enricher seam.
type DefaultEnricher struct {
	entityExtractor   EntityExtractor
	conceptExtractor  ConceptExtractor
	relationExtractor RelationExtractor
}

// NewEnricher constructs a DefaultEnricher using the HybridEntityExtractor by default.
// The hybrid extractor combines RuleBasedEntityExtractor (high-precision primary) with
// GLiNEREntityExtractor (high-recall secondary). If the GLiNER service is unavailable,
// extraction gracefully falls back to rule-based only.
func NewEnricher() *DefaultEnricher {
	ruleBased := NewRuleBasedEntityExtractor()
	gliner := NewGLiNEREntityExtractor("", 0) // uses defaults: localhost:8088, 3s timeout
	return &DefaultEnricher{
		entityExtractor: NewHybridEntityExtractor(ruleBased, gliner),
	}
}

// NewEnricherWithExtractors constructs a DefaultEnricher with injected extractor strategy providers.
func NewEnricherWithExtractors(ee EntityExtractor, ce ConceptExtractor, re RelationExtractor) *DefaultEnricher {
	return &DefaultEnricher{
		entityExtractor:   ee,
		conceptExtractor:  ce,
		relationExtractor: re,
	}
}

// Enrich executes multi-pass metadata enrichment using CompositeEnricher.
func (e *DefaultEnricher) Enrich(ctx context.Context, input *EnrichmentInput) *EnrichmentReport {
	if input == nil {
		return &EnrichmentReport{}
	}

	comp := NewCompositeEnricher([]EnricherPass{
		NewLanguageDetectionPass(),
		NewChunkStatisticsPass(),
		NewTitleAuthorPass(nil, nil),
		NewMetadataConsistencyPass(),
		NewPageResolutionPass(),
		NewEntityExtractorPass(e.entityExtractor),
		NewKeywordExtractorPass(nil),
		NewConceptExtractorPass(e.conceptExtractor),
		NewRelationExtractorPass(e.relationExtractor),
		NewSummaryPass(nil),
	})

	report, _ := comp.ExecutePasses(ctx, input)
	if report == nil {
		return &EnrichmentReport{}
	}

	return report
}

var multiSpaceRegex = regexp.MustCompile(`\s+`)

// NormalizeHeading converts a heading string into a canonical normalized string for resilient matching.
func NormalizeHeading(input string) string {
	if input == "" {
		return ""
	}

	// 1. Convert unicode apostrophes to standard single quotes
	s := strings.ReplaceAll(input, "’", "'")
	s = strings.ReplaceAll(s, "`", "'")

	// 2. Lowercase
	s = strings.ToLower(s)

	// 3. Keep letters, numbers, spaces, and single quotes, replace other punctuation with space
	var buf strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '\'' {
			buf.WriteRune(r)
		} else {
			buf.WriteRune(' ')
		}
	}

	// 4. Remove single quotes for pure word comparison if present
	result := strings.ReplaceAll(buf.String(), "'", "")

	// 5. Normalize multi-spaces
	result = multiSpaceRegex.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

func isGenericHeading(heading string) bool {
	norm := NormalizeHeading(heading)
	if norm == "" {
		return true
	}
	return genericHeadings[norm]
}

// EnrichSemanticTree resolves page numbers for SemanticTree nodes using PageMap and Chunk fallback provenance.
func EnrichSemanticTree(tree *model.SemanticTree, pageMap []model.PageMap, chunks []model.KnowledgeChunk) []string {
	if tree == nil || len(tree.RootNodes) == 0 {
		return nil
	}

	var warnings []string

	// Pre-index chunk section paths for fast fallback lookup
	chunkPageLookup := make(map[string][]int)
	for _, ch := range chunks {
		if ch.SectionPath != "" && len(ch.PageNumbers) > 0 {
			normSec := NormalizeHeading(ch.SectionPath)
			if _, exists := chunkPageLookup[normSec]; !exists {
				chunkPageLookup[normSec] = ch.PageNumbers
			}
		}
	}

	for i := range tree.RootNodes {
		w := enrichNode(&tree.RootNodes[i], pageMap, chunkPageLookup)
		if w != "" {
			warnings = append(warnings, w)
		}
	}

	return warnings
}

func enrichNode(node *model.SemanticNode, pageMap []model.PageMap, chunkPageLookup map[string][]int) string {
	if node == nil {
		return ""
	}

	normHeading := NormalizeHeading(node.Heading)
	resolvedPage := -1

	// Strategy A: Match explicit Markdown heading lines (# Heading, ## Heading, ### Heading)
	if normHeading != "" && len(pageMap) > 0 {
		for _, pm := range pageMap {
			if isExplicitHeadingOnPage(pm.Markdown, normHeading) {
				resolvedPage = pm.PageNumber
				break
			}
		}
	}

	// Strategy B: Match via Chunk provenance (chunks already tracked actual section body start pages!)
	if resolvedPage < 1 && normHeading != "" {
		if pgs, ok := chunkPageLookup[normHeading]; ok && len(pgs) > 0 {
			resolvedPage = pgs[0]
		}
	}

	// Strategy C: Plain text occurrence in PageMap (skipping TOC index pages)
	if resolvedPage < 1 && normHeading != "" && len(pageMap) > 0 {
		for _, pm := range pageMap {
			if isTOCPage(pm.Markdown) {
				continue
			}
			normPageMd := NormalizeHeading(pm.Markdown)
			if strings.Contains(normPageMd, normHeading) {
				resolvedPage = pm.PageNumber
				break
			}
		}
	}

	// Update node if resolved
	if resolvedPage >= 1 {
		node.PageNumbers = []int{resolvedPage}
	} else if len(node.PageNumbers) == 0 || (len(node.PageNumbers) == 1 && node.PageNumbers[0] == 1) {
		// Non-destructive fallback warning if page resolution fails
		return fmt.Sprintf("semantic page resolution unavailable for heading %q", node.Heading)
	}

	// Recursively enrich child nodes
	for i := range node.Children {
		enrichNode(&node.Children[i], pageMap, chunkPageLookup)
	}

	return ""
}

// isExplicitHeadingOnPage checks if a Markdown page contains an explicit heading line (# Heading, ## Heading, ### Heading) matching target heading.
func isExplicitHeadingOnPage(markdown, normHeading string) bool {
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			// Strip leading hashes
			headingText := strings.TrimLeft(trimmed, "#")
			headingText = strings.TrimSpace(headingText)
			if NormalizeHeading(headingText) == normHeading {
				return true
			}
		}
	}
	return false
}

// isTOCPage detects whether a page is part of Table of Contents / Index area.
func isTOCPage(markdown string) bool {
	norm := NormalizeHeading(markdown)
	if strings.Contains(norm, "table of contents") || strings.Contains(norm, "contents") || strings.Contains(norm, "78 areas of thought") {
		return true
	}
	// Check for dot leaders (e.g. ..... 6 or ... 71)
	if strings.Contains(markdown, ".....") || strings.Contains(markdown, ". . . .") {
		return true
	}
	return false
}
