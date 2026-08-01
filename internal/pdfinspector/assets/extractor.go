package assets

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"arca/internal/pdfinspector/model"
)

// Extractor defines the public interface for non-prose asset and citation extraction.
type Extractor interface {
	ExtractAssets(ctx context.Context, markdown string) (*model.Assets, error)
	ExtractAssetsWithContext(ctx context.Context, tree *model.SemanticTree, content *model.DocumentContent, chunks []model.KnowledgeChunk) (*model.Assets, error)
}

// AssetExtractor defines the interface for individual asset extraction components.
type AssetExtractor interface {
	Extract(ctx *AssetExtractionContext) (*model.ExtractedAssets, []model.ExtractionWarning, error)
}

// PageResolver defines the interface for resolving PDF page context from a source location.
type PageResolver interface {
	Resolve(location model.SourceLocation) model.PageContext
}

// SectionResolver defines the interface for resolving section path hierarchy from a source location.
type SectionResolver interface {
	Resolve(location model.SourceLocation) string
}

// ChunkResolver defines the interface for linking assets to overlapping KnowledgeChunk IDs.
type ChunkResolver interface {
	Resolve(location model.SourceLocation) []string
}

// AssetIDGenerator defines the interface for generating deterministic asset IDs.
type AssetIDGenerator interface {
	Generate(assetType model.AssetType) string
}

// AssetExtractionContext holds centralized text, hierarchy, layout, and positional state.
type AssetExtractionContext struct {
	Ctx         context.Context
	Markdown    string
	Tree        *model.SemanticTree
	Content     *model.DocumentContent
	Chunks      []model.KnowledgeChunk
	LineOffsets []int
	Lines       []string
}

// NewAssetExtractionContext constructs a fully initialized AssetExtractionContext.
func NewAssetExtractionContext(ctx context.Context, tree *model.SemanticTree, content *model.DocumentContent, chunks []model.KnowledgeChunk, markdown string) *AssetExtractionContext {
	if markdown == "" && content != nil {
		markdown = content.Markdown
	}
	lines := strings.Split(markdown, "\n")
	lineOffsets := make([]int, len(lines)+1)
	runningOffset := 0
	for i, l := range lines {
		lineOffsets[i] = runningOffset
		runningOffset += len(l) + 1 // +1 for newline character
	}
	lineOffsets[len(lines)] = runningOffset

	return &AssetExtractionContext{
		Ctx:         ctx,
		Markdown:    markdown,
		Tree:        tree,
		Content:     content,
		Chunks:      chunks,
		LineOffsets: lineOffsets,
		Lines:       lines,
	}
}

// LineFromOffset returns the 1-based line number for a given character byte offset.
func (c *AssetExtractionContext) LineFromOffset(offset int) int {
	if offset <= 0 || len(c.LineOffsets) == 0 {
		return 1
	}
	if offset >= c.LineOffsets[len(c.LineOffsets)-1] {
		return len(c.Lines)
	}
	idx := sort.Search(len(c.LineOffsets), func(i int) bool {
		return c.LineOffsets[i] > offset
	})
	if idx > 0 {
		return idx
	}
	return 1
}

// BuildLocation constructs a SourceLocation from start and end byte offsets.
func (c *AssetExtractionContext) BuildLocation(startOffset, endOffset int) model.SourceLocation {
	if startOffset < 0 {
		startOffset = 0
	}
	if endOffset < startOffset {
		endOffset = startOffset
	}
	if endOffset > len(c.Markdown) {
		endOffset = len(c.Markdown)
	}
	return model.SourceLocation{
		StartOffset: startOffset,
		EndOffset:   endOffset,
		StartLine:   c.LineFromOffset(startOffset),
		EndLine:     c.LineFromOffset(endOffset),
	}
}

// SequentialIDGenerator generates stable, deterministic asset IDs (tbl-1, fig-1, code-1, eq-1, cit-1).
type SequentialIDGenerator struct {
	mu       sync.Mutex
	counters map[model.AssetType]int
}

// NewSequentialIDGenerator initializes a new SequentialIDGenerator.
func NewSequentialIDGenerator() *SequentialIDGenerator {
	return &SequentialIDGenerator{
		counters: make(map[model.AssetType]int),
	}
}

// Generate returns the next deterministic ID for an asset type.
func (g *SequentialIDGenerator) Generate(assetType model.AssetType) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counters[assetType]++
	num := g.counters[assetType]

	var prefix string
	switch assetType {
	case model.AssetTypeTable:
		prefix = "tbl"
	case model.AssetTypeFigure:
		prefix = "fig"
	case model.AssetTypeCodeBlock:
		prefix = "code"
	case model.AssetTypeEquation:
		prefix = "eq"
	case model.AssetTypeCitation:
		prefix = "cit"
	default:
		prefix = "asset"
	}
	return fmt.Sprintf("%s-%d", prefix, num)
}

// DefaultPageResolver maps source locations to 1-based page numbers using DocumentContent.PageMap.
type DefaultPageResolver struct {
	content *model.DocumentContent
}

// NewDefaultPageResolver constructs a DefaultPageResolver instance.
func NewDefaultPageResolver(content *model.DocumentContent) *DefaultPageResolver {
	return &DefaultPageResolver{content: content}
}

// Resolve resolves the PageContext for a given SourceLocation.
func (r *DefaultPageResolver) Resolve(location model.SourceLocation) model.PageContext {
	if r.content == nil || len(r.content.PageMap) == 0 {
		return model.PageContext{PrimaryPage: 1, Pages: []int{1}}
	}

	markdown := r.content.Markdown
	var pageRanges []struct {
		page      int
		startByte int
		endByte   int
	}

	runningOffset := 0
	for _, pm := range r.content.PageMap {
		pgLen := len(pm.Markdown)
		if idx := strings.Index(markdown[runningOffset:], pm.Markdown); idx >= 0 {
			start := runningOffset + idx
			end := start + pgLen
			pageRanges = append(pageRanges, struct {
				page      int
				startByte int
				endByte   int
			}{page: pm.PageNumber, startByte: start, endByte: end})
			runningOffset = end
		} else {
			// Fallback estimation
			pageRanges = append(pageRanges, struct {
				page      int
				startByte int
				endByte   int
			}{page: pm.PageNumber, startByte: runningOffset, endByte: runningOffset + pgLen})
			runningOffset += pgLen
		}
	}

	matchingPages := make(map[int]bool)
	primaryPage := 0

	for _, pr := range pageRanges {
		if location.StartOffset <= pr.endByte && location.EndOffset >= pr.startByte {
			matchingPages[pr.page] = true
			if primaryPage == 0 {
				primaryPage = pr.page
			}
		}
	}

	if primaryPage == 0 {
		primaryPage = r.content.PageMap[0].PageNumber
		if primaryPage < 1 {
			primaryPage = 1
		}
	}

	pages := make([]int, 0, len(matchingPages))
	for p := range matchingPages {
		if p >= 1 {
			pages = append(pages, p)
		}
	}
	sort.Ints(pages)
	if len(pages) == 0 {
		pages = []int{primaryPage}
	}

	return model.PageContext{
		PrimaryPage: primaryPage,
		Pages:       pages,
	}
}

// DefaultSectionResolver resolves document section paths prioritizing SemanticTree, then Markdown headings, then fallback.
type DefaultSectionResolver struct {
	tree     *model.SemanticTree
	markdown string
	lines    []string
}

// NewDefaultSectionResolver constructs a DefaultSectionResolver instance.
func NewDefaultSectionResolver(tree *model.SemanticTree, markdown string) *DefaultSectionResolver {
	return &DefaultSectionResolver{
		tree:     tree,
		markdown: markdown,
		lines:    strings.Split(markdown, "\n"),
	}
}

// Resolve resolves the hierarchical section path string for a given SourceLocation.
func (r *DefaultSectionResolver) Resolve(location model.SourceLocation) string {
	// Priority 1: SemanticTree lookup if nodes present
	if r.tree != nil && len(r.tree.RootNodes) > 0 {
		if path := findPathInTree(r.tree.RootNodes, location.StartLine); path != "" {
			return path
		}
	}

	// Priority 2: Line-by-line Markdown heading tracker fallback
	headingRegex := regexp.MustCompile(`^(#{1,6})(\s+(.*)|$)`)
	type secHead struct {
		level int
		title string
	}
	var stack []secHead

	targetLine := location.StartLine
	if targetLine > len(r.lines) {
		targetLine = len(r.lines)
	}

	for i := 0; i < targetLine; i++ {
		line := strings.TrimSpace(r.lines[i])
		if match := headingRegex.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			title := strings.TrimSpace(match[2])
			if title == "" {
				title = "Untitled Section"
			}
			for len(stack) > 0 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, secHead{level: level, title: title})
		}
	}

	if len(stack) > 0 {
		parts := make([]string, len(stack))
		for i, s := range stack {
			parts[i] = s.title
		}
		return strings.Join(parts, " > ")
	}

	// Priority 3: Default Overview
	return "Document Overview"
}

func findPathInTree(nodes []model.SemanticNode, targetLine int) string {
	for _, n := range nodes {
		sub := findPathInNode(n, targetLine, []string{})
		if sub != "" {
			return sub
		}
	}
	return ""
}

func findPathInNode(n model.SemanticNode, targetLine int, parentPath []string) string {
	currentPath := append(append([]string{}, parentPath...), n.Heading)
	for _, child := range n.Children {
		if sub := findPathInNode(child, targetLine, currentPath); sub != "" {
			return sub
		}
	}
	return strings.Join(currentPath, " > ")
}

// OverlapChunkResolver correlates an asset's SourceLocation to overlapping KnowledgeChunk IDs.
type OverlapChunkResolver struct {
	chunks []model.KnowledgeChunk
}

// NewOverlapChunkResolver constructs an OverlapChunkResolver instance.
func NewOverlapChunkResolver(chunks []model.KnowledgeChunk) *OverlapChunkResolver {
	return &OverlapChunkResolver{chunks: chunks}
}

// Resolve resolves all overlapping KnowledgeChunk IDs for a given SourceLocation.
func (r *OverlapChunkResolver) Resolve(location model.SourceLocation) []string {
	if len(r.chunks) == 0 {
		return []string{}
	}

	var matched []string
	for _, chk := range r.chunks {
		if location.StartOffset <= chk.SourceOffsets.EndChar && location.EndOffset >= chk.SourceOffsets.StartChar {
			matched = append(matched, chk.ChunkID)
		}
	}
	if matched == nil {
		matched = []string{}
	}
	return matched
}

// Sub-Extractor: TableExtractor
type tableExtractor struct{}

func NewTableExtractor() AssetExtractor {
	return &tableExtractor{}
}

func (e *tableExtractor) Extract(ctx *AssetExtractionContext) (*model.ExtractedAssets, []model.ExtractionWarning, error) {
	res := &model.ExtractedAssets{
		Tables:     []model.Table{},
		Figures:    []model.Figure{},
		CodeBlocks: []model.CodeBlock{},
		Equations:  []model.Equation{},
		Citations:  []model.Citation{},
	}
	var warnings []model.ExtractionWarning

	select {
	case <-ctx.Ctx.Done():
		return nil, nil, ctx.Ctx.Err()
	default:
	}

	inTable := false
	var tableLines []string
	var startOffset int

	htmlTableRegex := regexp.MustCompile(`(?is)<table>(.*?)</table>`)

	// 1. GFM Markdown Tables
	for i, line := range ctx.Lines {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		trimmed := strings.TrimSpace(line)
		isTableRow := strings.Contains(line, "|") && (strings.HasPrefix(trimmed, "|") || strings.HasSuffix(trimmed, "|"))

		if isTableRow {
			if !inTable {
				inTable = true
				startOffset = ctx.LineOffsets[i]
				tableLines = []string{line}
			} else {
				tableLines = append(tableLines, line)
			}
		} else if inTable {
			endOffset := ctx.LineOffsets[i]
			content := strings.Join(tableLines, "\n")
			loc := ctx.BuildLocation(startOffset, endOffset)

			headers := parseTableHeaders(tableLines[0])
			caption := extractPrecedingCaption(ctx.Lines, ctx.LineFromOffset(startOffset)-1)

			res.Tables = append(res.Tables, model.Table{
				AssetMetadata: model.AssetMetadata{
					AssetType:      model.AssetTypeTable,
					SourceLocation: loc,
				},
				Caption: caption,
				Content: content,
				Headers: headers,
			})

			inTable = false
			tableLines = nil
		}
	}

	if inTable {
		endOffset := ctx.LineOffsets[len(ctx.Lines)]
		content := strings.Join(tableLines, "\n")
		loc := ctx.BuildLocation(startOffset, endOffset)
		headers := parseTableHeaders(tableLines[0])
		caption := extractPrecedingCaption(ctx.Lines, ctx.LineFromOffset(startOffset)-1)

		res.Tables = append(res.Tables, model.Table{
			AssetMetadata: model.AssetMetadata{
				AssetType:      model.AssetTypeTable,
				SourceLocation: loc,
			},
			Caption: caption,
			Content: content,
			Headers: headers,
		})
	}

	// 2. HTML <table> blocks
	htmlMatches := htmlTableRegex.FindAllStringIndex(ctx.Markdown, -1)
	for _, m := range htmlMatches {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		loc := ctx.BuildLocation(m[0], m[1])
		content := ctx.Markdown[m[0]:m[1]]
		if !strings.Contains(content, "</table>") {
			warnings = append(warnings, model.ExtractionWarning{
				Message:        "malformed HTML table block encountered",
				Severity:       model.SeverityWarning,
				SourceLocation: loc,
			})
			continue
		}
		res.Tables = append(res.Tables, model.Table{
			AssetMetadata: model.AssetMetadata{
				AssetType:      model.AssetTypeTable,
				SourceLocation: loc,
			},
			Caption: extractPrecedingCaption(ctx.Lines, loc.StartLine-1),
			Content: content,
			Headers: []string{},
		})
	}

	return res, warnings, nil
}

func parseTableHeaders(headerLine string) []string {
	parts := strings.Split(headerLine, "|")
	var headers []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			headers = append(headers, trimmed)
		}
	}
	if headers == nil {
		headers = []string{}
	}
	return headers
}

func extractPrecedingCaption(lines []string, prevLineIdx int) string {
	if prevLineIdx < 1 || prevLineIdx > len(lines) {
		return ""
	}
	line := strings.TrimSpace(lines[prevLineIdx-1])
	lowered := strings.ToLower(line)
	if strings.HasPrefix(lowered, "table") || strings.HasPrefix(lowered, "caption:") || strings.HasPrefix(lowered, "**table") {
		return line
	}
	return ""
}

// Sub-Extractor: FigureExtractor
type figureExtractor struct{}

func NewFigureExtractor() AssetExtractor {
	return &figureExtractor{}
}

func (e *figureExtractor) Extract(ctx *AssetExtractionContext) (*model.ExtractedAssets, []model.ExtractionWarning, error) {
	res := &model.ExtractedAssets{
		Tables:     []model.Table{},
		Figures:    []model.Figure{},
		CodeBlocks: []model.CodeBlock{},
		Equations:  []model.Equation{},
		Citations:  []model.Citation{},
	}
	var warnings []model.ExtractionWarning

	mdImgRegex := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	htmlImgRegex := regexp.MustCompile(`(?i)<img\s+[^>]*src=["']([^"']+)["'][^>]*>`)

	// 1. Markdown Images
	matches := mdImgRegex.FindAllStringSubmatchIndex(ctx.Markdown, -1)
	for _, m := range matches {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		loc := ctx.BuildLocation(m[0], m[1])
		caption := ctx.Markdown[m[2]:m[3]]
		uri := ctx.Markdown[m[4]:m[5]]
		if idx := strings.Index(uri, " "); idx > 0 {
			uri = strings.TrimSpace(uri[:idx])
		}

		res.Figures = append(res.Figures, model.Figure{
			AssetMetadata: model.AssetMetadata{
				AssetType:      model.AssetTypeFigure,
				SourceLocation: loc,
			},
			Caption: caption,
			URI:     uri,
		})
	}

	// 2. HTML <img> Tags
	htmlMatches := htmlImgRegex.FindAllStringSubmatchIndex(ctx.Markdown, -1)
	for _, m := range htmlMatches {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		loc := ctx.BuildLocation(m[0], m[1])
		uri := ctx.Markdown[m[2]:m[3]]

		// Extract alt attribute if present
		tagStr := ctx.Markdown[m[0]:m[1]]
		alt := ""
		altRegex := regexp.MustCompile(`(?i)alt=["']([^"']+)["']`)
		if altMatch := altRegex.FindStringSubmatch(tagStr); altMatch != nil {
			alt = altMatch[1]
		}

		res.Figures = append(res.Figures, model.Figure{
			AssetMetadata: model.AssetMetadata{
				AssetType:      model.AssetTypeFigure,
				SourceLocation: loc,
			},
			Caption: alt,
			URI:     uri,
		})
	}

	// 3. Malformed/Unclosed HTML <img> Tag Detection
	for i, line := range ctx.Lines {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}
		trimmed := strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(trimmed), "<img") && !strings.Contains(trimmed, ">") {
			startOffset := ctx.LineOffsets[i]
			endOffset := startOffset + len(line)
			loc := ctx.BuildLocation(startOffset, endOffset)
			warnings = append(warnings, model.ExtractionWarning{
				Message:        fmt.Sprintf("malformed HTML image tag encountered: %q", trimmed),
				Severity:       model.SeverityWarning,
				SourceLocation: loc,
			})
		}
	}

	return res, warnings, nil
}

// Sub-Extractor: CodeExtractor
type codeExtractor struct{}

func NewCodeExtractor() AssetExtractor {
	return &codeExtractor{}
}

func (e *codeExtractor) Extract(ctx *AssetExtractionContext) (*model.ExtractedAssets, []model.ExtractionWarning, error) {
	res := &model.ExtractedAssets{
		Tables:     []model.Table{},
		Figures:    []model.Figure{},
		CodeBlocks: []model.CodeBlock{},
		Equations:  []model.Equation{},
		Citations:  []model.Citation{},
	}
	var warnings []model.ExtractionWarning

	inCode := false
	codeFence := ""
	language := ""
	var codeLines []string
	var startOffset int

	for i, line := range ctx.Lines {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence := trimmed[:3]
			if !inCode {
				inCode = true
				codeFence = fence
				language = strings.TrimSpace(trimmed[3:])
				startOffset = ctx.LineOffsets[i]
				codeLines = []string{}
			} else if strings.HasPrefix(trimmed, codeFence) {
				endOffset := ctx.LineOffsets[i] + len(line)
				loc := ctx.BuildLocation(startOffset, endOffset)
				content := strings.Join(codeLines, "\n")

				res.CodeBlocks = append(res.CodeBlocks, model.CodeBlock{
					AssetMetadata: model.AssetMetadata{
						AssetType:      model.AssetTypeCodeBlock,
						SourceLocation: loc,
					},
					Language: language,
					Content:  content,
				})

				inCode = false
				codeFence = ""
				language = ""
				codeLines = nil
			} else {
				codeLines = append(codeLines, line)
			}
		} else if inCode {
			codeLines = append(codeLines, line)
		}
	}

	if inCode {
		endOffset := ctx.LineOffsets[len(ctx.Lines)]
		loc := ctx.BuildLocation(startOffset, endOffset)
		content := strings.Join(codeLines, "\n")
		res.CodeBlocks = append(res.CodeBlocks, model.CodeBlock{
			AssetMetadata: model.AssetMetadata{
				AssetType:      model.AssetTypeCodeBlock,
				SourceLocation: loc,
			},
			Language: language,
			Content:  content,
		})
	}

	return res, warnings, nil
}

// Sub-Extractor: EquationExtractor
type equationExtractor struct{}

func NewEquationExtractor() AssetExtractor {
	return &equationExtractor{}
}

func (e *equationExtractor) Extract(ctx *AssetExtractionContext) (*model.ExtractedAssets, []model.ExtractionWarning, error) {
	res := &model.ExtractedAssets{
		Tables:     []model.Table{},
		Figures:    []model.Figure{},
		CodeBlocks: []model.CodeBlock{},
		Equations:  []model.Equation{},
		Citations:  []model.Citation{},
	}
	var warnings []model.ExtractionWarning

	// Block math: $$ ... $$ or \[ ... \]
	blockMathRegex := regexp.MustCompile(`(?s)\$\$(.*?)\$\$|\\\[(.*?)\\\]`)

	matches := blockMathRegex.FindAllStringSubmatchIndex(ctx.Markdown, -1)
	for _, m := range matches {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		loc := ctx.BuildLocation(m[0], m[1])
		var latex string
		if m[2] != -1 {
			latex = strings.TrimSpace(ctx.Markdown[m[2]:m[3]])
		} else if m[4] != -1 {
			latex = strings.TrimSpace(ctx.Markdown[m[4]:m[5]])
		}

		res.Equations = append(res.Equations, model.Equation{
			AssetMetadata: model.AssetMetadata{
				AssetType:      model.AssetTypeEquation,
				SourceLocation: loc,
			},
			LaTeX: latex,
		})
	}

	return res, warnings, nil
}

// Sub-Extractor: CitationExtractor
type citationExtractor struct{}

func NewCitationExtractor() AssetExtractor {
	return &citationExtractor{}
}

func (e *citationExtractor) Extract(ctx *AssetExtractionContext) (*model.ExtractedAssets, []model.ExtractionWarning, error) {
	res := &model.ExtractedAssets{
		Tables:     []model.Table{},
		Figures:    []model.Figure{},
		CodeBlocks: []model.CodeBlock{},
		Equations:  []model.Equation{},
		Citations:  []model.Citation{},
	}
	var warnings []model.ExtractionWarning

	seenOffsets := make(map[int]bool)

	// 1. Bibliography Sections (Items under # References or # Bibliography)
	inBibSection := false
	bibHeadingRegex := regexp.MustCompile(`(?i)^(#{1,6})\s+(references|bibliography)`)
	bibItemRegex := regexp.MustCompile(`(?m)^(?:\d+\.|\*|\[\d+\]|\[[^\]]+\])\s+(.+)`)

	for i, line := range ctx.Lines {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		trimmed := strings.TrimSpace(line)
		if bibHeadingRegex.MatchString(trimmed) {
			inBibSection = true
			continue
		}

		if inBibSection {
			if strings.HasPrefix(trimmed, "#") {
				inBibSection = false
				continue
			}

			if trimmed != "" && bibItemRegex.MatchString(trimmed) {
				startOffset := ctx.LineOffsets[i]
				endOffset := startOffset + len(line)
				loc := ctx.BuildLocation(startOffset, endOffset)
				seenOffsets[startOffset] = true

				res.Citations = append(res.Citations, model.Citation{
					AssetMetadata: model.AssetMetadata{
						AssetType:      model.AssetTypeCitation,
						SourceLocation: loc,
					},
					RawText:      trimmed,
					CitationType: model.CitationTypeBibliography,
				})
			}
		}
	}

	// 2. Footnotes ([^1]: ... or [^note])
	footnoteDefRegex := regexp.MustCompile(`\[\^([^\]]+)\]:\s*(.+)`)
	footnoteRefRegex := regexp.MustCompile(`\[\^([^\]]+)\]`)

	for i, line := range ctx.Lines {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		trimmed := strings.TrimSpace(line)
		startOffset := ctx.LineOffsets[i]

		if footnoteDefRegex.MatchString(trimmed) {
			if !seenOffsets[startOffset] {
				seenOffsets[startOffset] = true
				loc := ctx.BuildLocation(startOffset, startOffset+len(line))
				res.Citations = append(res.Citations, model.Citation{
					AssetMetadata: model.AssetMetadata{
						AssetType:      model.AssetTypeCitation,
						SourceLocation: loc,
					},
					RawText:      trimmed,
					CitationType: model.CitationTypeFootnote,
				})
			}
		} else if matches := footnoteRefRegex.FindAllStringIndex(line, -1); len(matches) > 0 {
			for _, m := range matches {
				absStart := startOffset + m[0]
				absEnd := startOffset + m[1]
				if !seenOffsets[absStart] {
					seenOffsets[absStart] = true
					loc := ctx.BuildLocation(absStart, absEnd)
					res.Citations = append(res.Citations, model.Citation{
						AssetMetadata: model.AssetMetadata{
							AssetType:      model.AssetTypeCitation,
							SourceLocation: loc,
						},
						RawText:      line[m[0]:m[1]],
						CitationType: model.CitationTypeFootnote,
					})
				}
			}
		}
	}

	// 3. Catalog Metadata (ISBN, LCCN, Copyright ©)
	catalogRegex := regexp.MustCompile(`(?i)(ISBN(?:-13|-10)?:\s*[\d\-X]+|LCCN\s*[\d]+|Copyright\s*©\s*[\d]{4}[^\n]*)`)
	catMatches := catalogRegex.FindAllStringIndex(ctx.Markdown, -1)
	for _, m := range catMatches {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		if !seenOffsets[m[0]] {
			seenOffsets[m[0]] = true
			loc := ctx.BuildLocation(m[0], m[1])
			res.Citations = append(res.Citations, model.Citation{
				AssetMetadata: model.AssetMetadata{
					AssetType:      model.AssetTypeCitation,
					SourceLocation: loc,
				},
				RawText:      strings.TrimSpace(ctx.Markdown[m[0]:m[1]]),
				CitationType: model.CitationTypeAttribution,
			})
		}
	}

	// 4. Inline Citations ([1], [Smith et al., 2020])
	inlineCitRegex := regexp.MustCompile(`\[(\d+|[A-Za-z]+\s+et\s+al\.,?\s*\d{4})\]`)
	inlineMatches := inlineCitRegex.FindAllStringIndex(ctx.Markdown, -1)
	for _, m := range inlineMatches {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		if !seenOffsets[m[0]] {
			seenOffsets[m[0]] = true
			loc := ctx.BuildLocation(m[0], m[1])
			res.Citations = append(res.Citations, model.Citation{
				AssetMetadata: model.AssetMetadata{
					AssetType:      model.AssetTypeCitation,
					SourceLocation: loc,
				},
				RawText:      ctx.Markdown[m[0]:m[1]],
				CitationType: model.CitationTypeInline,
			})
		}
	}

	// 5. Epigraph Attributions (### Author heading following quote or Excerpt from...)
	attrRegex := regexp.MustCompile(`(?i)Excerpt\s+from\s+[^\n]+|As\s+told\s+to\s+[^\n]+`)
	attrMatches := attrRegex.FindAllStringIndex(ctx.Markdown, -1)
	for _, m := range attrMatches {
		select {
		case <-ctx.Ctx.Done():
			return nil, nil, ctx.Ctx.Err()
		default:
		}

		if !seenOffsets[m[0]] {
			seenOffsets[m[0]] = true
			loc := ctx.BuildLocation(m[0], m[1])
			res.Citations = append(res.Citations, model.Citation{
				AssetMetadata: model.AssetMetadata{
					AssetType:      model.AssetTypeCitation,
					SourceLocation: loc,
				},
				RawText:      strings.TrimSpace(ctx.Markdown[m[0]:m[1]]),
				CitationType: model.CitationTypeAttribution,
			})
		}
	}

	return res, warnings, nil
}

// PipelineExtractor orchestrates registered AssetExtractor plugins, metadata resolution, ordering, and stats computation.
type PipelineExtractor struct {
	mu         sync.RWMutex
	extractors []AssetExtractor
}

// NewExtractor creates a standard PipelineExtractor pre-loaded with default extractors.
func NewExtractor() *PipelineExtractor {
	p := &PipelineExtractor{
		extractors: make([]AssetExtractor, 0),
	}
	p.Register(NewTableExtractor())
	p.Register(NewFigureExtractor())
	p.Register(NewCodeExtractor())
	p.Register(NewEquationExtractor())
	p.Register(NewCitationExtractor())
	return p
}

// NewPipelineExtractor constructs an empty PipelineExtractor.
func NewPipelineExtractor() *PipelineExtractor {
	return &PipelineExtractor{
		extractors: make([]AssetExtractor, 0),
	}
}

// Register registers an AssetExtractor into the pipeline.
func (p *PipelineExtractor) Register(extractor AssetExtractor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.extractors = append(p.extractors, extractor)
}

// ExtractAssets executes extraction over raw Markdown text without tree or content context.
func (p *PipelineExtractor) ExtractAssets(ctx context.Context, markdown string) (*model.Assets, error) {
	return p.ExtractAssetsWithContext(ctx, nil, &model.DocumentContent{Markdown: markdown}, nil)
}

// ExtractAssetsWithContext executes the full extraction pipeline with tree, page map, and chunk context.
func (p *PipelineExtractor) ExtractAssetsWithContext(ctx context.Context, tree *model.SemanticTree, content *model.DocumentContent, chunks []model.KnowledgeChunk) (*model.Assets, error) {
	start := time.Now()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	markdown := ""
	if content != nil {
		markdown = content.Markdown
	}

	actx := NewAssetExtractionContext(ctx, tree, content, chunks, markdown)

	p.mu.RLock()
	pipeline := make([]AssetExtractor, len(p.extractors))
	copy(pipeline, p.extractors)
	p.mu.RUnlock()

	merged := &model.ExtractedAssets{
		Tables:     []model.Table{},
		Figures:    []model.Figure{},
		CodeBlocks: []model.CodeBlock{},
		Equations:  []model.Equation{},
		Citations:  []model.Citation{},
	}
	var allWarnings []model.ExtractionWarning

	for _, ext := range pipeline {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		subAssets, warnings, err := ext.Extract(actx)
		if err != nil {
			return nil, err
		}
		if warnings != nil {
			allWarnings = append(allWarnings, warnings...)
		}
		if subAssets != nil {
			merged.Tables = append(merged.Tables, subAssets.Tables...)
			merged.Figures = append(merged.Figures, subAssets.Figures...)
			merged.CodeBlocks = append(merged.CodeBlocks, subAssets.CodeBlocks...)
			merged.Equations = append(merged.Equations, subAssets.Equations...)
			merged.Citations = append(merged.Citations, subAssets.Citations...)
		}
	}

	// Initialize Resolvers & ID Generator
	idGen := NewSequentialIDGenerator()
	pageResolver := NewDefaultPageResolver(content)
	sectionResolver := NewDefaultSectionResolver(tree, actx.Markdown)
	chunkResolver := NewOverlapChunkResolver(chunks)

	// Collect all concrete assets into generic slice for metadata resolution and ordering
	var assetList []model.Asset

	for i := range merged.Tables {
		merged.Tables[i].ID = idGen.Generate(model.AssetTypeTable)
		merged.Tables[i].AssetType = model.AssetTypeTable
		assetList = append(assetList, &merged.Tables[i])
	}
	for i := range merged.Figures {
		merged.Figures[i].ID = idGen.Generate(model.AssetTypeFigure)
		merged.Figures[i].AssetType = model.AssetTypeFigure
		assetList = append(assetList, &merged.Figures[i])
	}
	for i := range merged.CodeBlocks {
		merged.CodeBlocks[i].ID = idGen.Generate(model.AssetTypeCodeBlock)
		merged.CodeBlocks[i].AssetType = model.AssetTypeCodeBlock
		assetList = append(assetList, &merged.CodeBlocks[i])
	}
	for i := range merged.Equations {
		merged.Equations[i].ID = idGen.Generate(model.AssetTypeEquation)
		merged.Equations[i].AssetType = model.AssetTypeEquation
		assetList = append(assetList, &merged.Equations[i])
	}
	for i := range merged.Citations {
		merged.Citations[i].ID = idGen.Generate(model.AssetTypeCitation)
		merged.Citations[i].AssetType = model.AssetTypeCitation
		assetList = append(assetList, &merged.Citations[i])
	}

	// Sort assets strictly by SourceLocation.StartOffset to preserve original document flow
	sort.SliceStable(assetList, func(i, j int) bool {
		return assetList[i].GetMetadata().SourceLocation.StartOffset < assetList[j].GetMetadata().SourceLocation.StartOffset
	})

	orderedRefs := make([]model.AssetReference, 0, len(assetList))
	for _, ast := range assetList {
		meta := ast.GetMetadata()

		pgCtx := pageResolver.Resolve(meta.SourceLocation)
		secPath := sectionResolver.Resolve(meta.SourceLocation)
		relChunks := chunkResolver.Resolve(meta.SourceLocation)

		// Enrich metadata on concrete target asset
		switch a := ast.(type) {
		case *model.Table:
			a.PageNumber = pgCtx.PrimaryPage
			a.PageNumbers = pgCtx.Pages
			a.SectionPath = secPath
			a.RelatedChunkIDs = relChunks
		case *model.Figure:
			a.PageNumber = pgCtx.PrimaryPage
			a.PageNumbers = pgCtx.Pages
			a.SectionPath = secPath
			a.RelatedChunkIDs = relChunks
		case *model.CodeBlock:
			a.PageNumber = pgCtx.PrimaryPage
			a.PageNumbers = pgCtx.Pages
			a.SectionPath = secPath
			a.RelatedChunkIDs = relChunks
		case *model.Equation:
			a.PageNumber = pgCtx.PrimaryPage
			a.PageNumbers = pgCtx.Pages
			a.SectionPath = secPath
			a.RelatedChunkIDs = relChunks
		case *model.Citation:
			a.PageNumber = pgCtx.PrimaryPage
			a.PageNumbers = pgCtx.Pages
			a.SectionPath = secPath
			a.RelatedChunkIDs = relChunks
		}

		orderedRefs = append(orderedRefs, model.AssetReference{
			ID:             meta.ID,
			AssetType:      meta.AssetType,
			SourceLocation: meta.SourceLocation,
		})
	}

	if allWarnings == nil {
		allWarnings = []model.ExtractionWarning{}
	}

	stats := model.ExtractionStats{
		TablesFound:     len(merged.Tables),
		FiguresFound:    len(merged.Figures),
		CodeBlocksFound: len(merged.CodeBlocks),
		EquationsFound:  len(merged.Equations),
		CitationsFound:  len(merged.Citations),
		WarningCount:    len(allWarnings),
		DurationMs:      time.Since(start).Milliseconds(),
	}

	return &model.Assets{
		Tables:     merged.Tables,
		Figures:    merged.Figures,
		CodeBlocks: merged.CodeBlocks,
		Equations:  merged.Equations,
		Citations:  merged.Citations,
		Warnings:   allWarnings,
		Ordered:    orderedRefs,
		Stats:      stats,
	}, nil
}
