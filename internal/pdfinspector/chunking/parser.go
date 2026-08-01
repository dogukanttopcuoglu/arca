package chunking

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"arca/internal/pdfinspector/model"
)

// BlockParser parses a document Markdown string and SemanticTree into an intermediate sequence of SemanticBlock structures.
type BlockParser interface {
	Parse(ctx context.Context, tree *model.SemanticTree, markdown string) ([]SemanticBlock, error)
}

// DefaultBlockParser implements BlockParser.
type DefaultBlockParser struct{}

// NewBlockParser creates a new DefaultBlockParser.
func NewBlockParser() *DefaultBlockParser {
	return &DefaultBlockParser{}
}

var (
	parserPageMarkerRegex = regexp.MustCompile(`(?i)<!--\s*page:?\s*(\d+)\s*-->|(?i)^---\s*page\s*(\d+)\s*---|(?i)\[page\s*(\d+)\]`)
	parserPageBreakRegex  = regexp.MustCompile(`(?i)<!--\s*pagebreak\s*-->`)
	parserHeadingRegex    = regexp.MustCompile(`^(#{1,6})(\s+(.*)|$)`)
	citationRegex         = regexp.MustCompile(`\[(\d+|[A-Za-z]+\s+et\s+al\.,?\s*\d{4})\]`)
)

// Parse converts Markdown and SemanticTree into an ordered slice of SemanticBlock elements.
func (p *DefaultBlockParser) Parse(ctx context.Context, tree *model.SemanticTree, markdown string) ([]SemanticBlock, error) {
	if markdown == "" {
		return []SemanticBlock{}, nil
	}

	lines := strings.Split(markdown, "\n")
	currentPage := 1

	// Build section path map or stack tracking
	currentSectionPath := "Document Overview"
	currentHeadingLevel := 1

	var blocks []SemanticBlock
	var currentBlockLines []string
	var currentBlockStartChar int
	var currentKind BlockKind
	var currentCategory model.SemanticCategory

	lineCharOffsets := make([]int, len(lines)+1)
	runningOffset := 0
	for i, l := range lines {
		lineCharOffsets[i] = runningOffset
		runningOffset += len(l) + 1 // +1 for \n
	}
	lineCharOffsets[len(lines)] = runningOffset

	inCodeBlock := false
	codeBlockFence := ""
	inEquationBlock := false

	flushCurrentBlock := func(endLineIdx int) {
		if len(currentBlockLines) == 0 {
			return
		}
		content := strings.Join(currentBlockLines, "\n")
		trimmed := strings.TrimSpace(content)
		if trimmed == "" {
			currentBlockLines = nil
			return
		}

		endChar := lineCharOffsets[endLineIdx]
		if endChar > len(markdown) {
			endChar = len(markdown)
		}
		if currentBlockStartChar > endChar {
			currentBlockStartChar = endChar
		}

		cits := extractCitations(content, currentPage)

		blocks = append(blocks, SemanticBlock{
			Kind:             currentKind,
			HeadingLevel:     currentHeadingLevel,
			SectionPath:      currentSectionPath,
			Markdown:         content,
			PageNumbers:      []int{currentPage},
			SourceOffsets:    model.SourceOffset{StartChar: currentBlockStartChar, EndChar: endChar},
			Citations:        cits,
			SemanticCategory: currentCategory,
		})

		currentBlockLines = nil
		currentKind = KindParagraph
		currentCategory = model.SemanticNarrative
	}

	type sectionHead struct {
		level int
		title string
	}
	var sectionStack []sectionHead

	buildSectionPath := func() string {
		if len(sectionStack) == 0 {
			return "Document Overview"
		}
		parts := make([]string, len(sectionStack))
		for i, s := range sectionStack {
			parts[i] = s.title
		}
		return strings.Join(parts, " > ")
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check page markers
		if pageMatch := parserPageMarkerRegex.FindStringSubmatch(trimmed); pageMatch != nil {
			flushCurrentBlock(i)
			for _, g := range pageMatch[1:] {
				if g != "" {
					if pg, err := strconv.Atoi(g); err == nil && pg > 0 {
						currentPage = pg
						break
					}
				}
			}
			continue
		}

		if parserPageBreakRegex.MatchString(trimmed) || strings.Contains(line, "\f") {
			flushCurrentBlock(i)
			currentPage++
			continue
		}

		// Handle Fenced Code Blocks
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence := trimmed[:3]
			if !inCodeBlock {
				flushCurrentBlock(i)
				inCodeBlock = true
				codeBlockFence = fence
				currentKind = KindCode
				currentCategory = model.SemanticCode
				currentBlockStartChar = lineCharOffsets[i]
				currentBlockLines = append(currentBlockLines, line)
				continue
			} else if strings.HasPrefix(trimmed, codeBlockFence) {
				currentBlockLines = append(currentBlockLines, line)
				flushCurrentBlock(i + 1)
				inCodeBlock = false
				codeBlockFence = ""
				continue
			}
		}

		if inCodeBlock {
			currentBlockLines = append(currentBlockLines, line)
			continue
		}

		// Heading Detection
		if headMatch := parserHeadingRegex.FindStringSubmatch(trimmed); headMatch != nil {
			flushCurrentBlock(i)
			level := len(headMatch[1])
			headingText := strings.TrimSpace(headMatch[2])
			if headingText == "" {
				headingText = "Untitled Section"
			}

			// Pop stack until parent level < current level
			for len(sectionStack) > 0 && sectionStack[len(sectionStack)-1].level >= level {
				sectionStack = sectionStack[:len(sectionStack)-1]
			}
			sectionStack = append(sectionStack, sectionHead{level: level, title: headingText})
			currentSectionPath = buildSectionPath()
			currentHeadingLevel = level
			continue
		}

		// Empty line flushes paragraph / element block if not inside table or list
		if trimmed == "" {
			if currentKind != KindTable && currentKind != KindList {
				flushCurrentBlock(i)
			}
			continue
		}

		// Handle Equation Block state
		if inEquationBlock {
			currentBlockLines = append(currentBlockLines, line)
			if strings.HasSuffix(trimmed, "$$") || strings.HasSuffix(trimmed, "\\]") {
				flushCurrentBlock(i + 1)
				inEquationBlock = false
			}
			continue
		}

		// Math Equation Display Block ($$ or \[)
		if strings.HasPrefix(trimmed, "$$") || strings.HasPrefix(trimmed, "\\[") {
			flushCurrentBlock(i)
			currentKind = KindEquation
			currentCategory = model.SemanticEquation
			currentBlockStartChar = lineCharOffsets[i]
			currentBlockLines = append(currentBlockLines, line)

			if (strings.HasSuffix(trimmed, "$$") && len(trimmed) > 2) || (strings.HasSuffix(trimmed, "\\]") && len(trimmed) > 2) {
				flushCurrentBlock(i + 1)
			} else {
				inEquationBlock = true
			}
			continue
		}

		// Table Row Detection
		if strings.Contains(line, "|") && (strings.HasPrefix(trimmed, "|") || strings.HasSuffix(trimmed, "|")) {
			if currentKind != KindTable {
				flushCurrentBlock(i)
				currentKind = KindTable
				currentCategory = model.SemanticTable
				currentBlockStartChar = lineCharOffsets[i]
			}
			currentBlockLines = append(currentBlockLines, line)
			continue
		} else if currentKind == KindTable {
			flushCurrentBlock(i)
		}

		// Figure Detection
		if strings.HasPrefix(trimmed, "![") || strings.HasPrefix(trimmed, "<!-- figure") {
			flushCurrentBlock(i)
			currentKind = KindFigure
			currentCategory = model.SemanticFigure
			currentBlockStartChar = lineCharOffsets[i]
			currentBlockLines = append(currentBlockLines, line)
			flushCurrentBlock(i + 1)
			continue
		}

		// List Item Detection
		if isListItem(trimmed) {
			if currentKind != KindList {
				flushCurrentBlock(i)
				currentKind = KindList
				currentCategory = model.SemanticProcedure
				currentBlockStartChar = lineCharOffsets[i]
			}
			currentBlockLines = append(currentBlockLines, line)
			continue
		} else if currentKind == KindList {
			flushCurrentBlock(i)
		}

		// Normal Paragraph
		if currentKind != KindParagraph {
			flushCurrentBlock(i)
			currentKind = KindParagraph
			currentCategory = classifyParagraphCategory(trimmed)
			currentBlockStartChar = lineCharOffsets[i]
		}
		currentBlockLines = append(currentBlockLines, line)
	}

	flushCurrentBlock(len(lines))
	return blocks, nil
}

func isListItem(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	if len(line) > 2 && line[0] >= '0' && line[0] <= '9' {
		idx := strings.Index(line, ". ")
		if idx > 0 && idx < 5 {
			return true
		}
	}
	return false
}

func classifyParagraphCategory(text string) model.SemanticCategory {
	lowered := strings.ToLower(text)
	if strings.HasPrefix(lowered, "note:") || strings.HasPrefix(lowered, "warning:") || strings.HasPrefix(lowered, "caution:") || strings.HasPrefix(lowered, "important:") {
		return model.SemanticWarning
	}
	if strings.Contains(lowered, "is defined as") || strings.Contains(lowered, "refers to") || strings.HasPrefix(lowered, "definition:") {
		return model.SemanticDefinition
	}
	if strings.HasPrefix(lowered, "for example:") || strings.HasPrefix(lowered, "e.g.,") || strings.HasPrefix(lowered, "example:") {
		return model.SemanticExample
	}
	if strings.HasPrefix(lowered, "see ") || strings.HasPrefix(lowered, "refer to ") {
		return model.SemanticReference
	}
	return model.SemanticNarrative
}

func extractCitations(content string, page int) []model.Citation {
	matches := citationRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return []model.Citation{}
	}
	cits := make([]model.Citation, 0, len(matches))
	seen := make(map[string]bool)
	for i, m := range matches {
		raw := m[0]
		if seen[raw] {
			continue
		}
		seen[raw] = true
		cits = append(cits, model.Citation{
			AssetMetadata: model.AssetMetadata{
				ID:         fmt.Sprintf("cit-%d", i+1),
				AssetType:  model.AssetTypeCitation,
				PageNumber: page,
			},
			RawText:      raw,
			CitationType: model.CitationTypeInline,
		})
	}
	return cits
}
