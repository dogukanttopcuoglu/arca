package semantic

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"arca/internal/pdfinspector/model"
)

// Processor defines the interface for converting raw Firecrawl output into a SemanticTree structure.
type Processor interface {
	ProcessExtraction(ctx context.Context, raw *model.RawExtractionResult) (*model.SemanticTree, error)
}

// DefaultProcessor implements Processor with thread-safe diagnostic warning collection.
type DefaultProcessor struct {
	mu       sync.RWMutex
	warnings []string
}

// NewProcessor creates a new SemanticProcessor.
func NewProcessor() *DefaultProcessor {
	return &DefaultProcessor{
		warnings: make([]string, 0),
	}
}

// Warnings returns a copy of diagnostic warnings emitted during extraction processing.
func (p *DefaultProcessor) Warnings() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cp := make([]string, len(p.warnings))
	copy(cp, p.warnings)
	return cp
}

func (p *DefaultProcessor) addWarning(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.warnings = append(p.warnings, msg)
}

type nodeBuilder struct {
	ID          string
	Heading     string
	Level       int
	PageNumbers []int
	Children    []*nodeBuilder
}

func (b *nodeBuilder) addPage(pg int) {
	if pg < 1 {
		return
	}
	for _, p := range b.PageNumbers {
		if p == pg {
			return
		}
	}
	b.PageNumbers = append(b.PageNumbers, pg)
}

func convertBuilder(b *nodeBuilder) model.SemanticNode {
	children := make([]model.SemanticNode, 0, len(b.Children))
	for _, child := range b.Children {
		children = append(children, convertBuilder(child))
	}
	return model.SemanticNode{
		ID:          b.ID,
		Heading:     b.Heading,
		Level:       b.Level,
		PageNumbers: b.PageNumbers,
		Children:    children,
	}
}

var (
	pageMarkerRegex = regexp.MustCompile(`(?i)<!--\s*page:?\s*(\d+)\s*-->|(?i)^---\s*page\s*(\d+)\s*---|(?i)\[page\s*(\d+)\]`)
	pageBreakRegex  = regexp.MustCompile(`(?i)<!--\s*pagebreak\s*-->`)
	headingRegex    = regexp.MustCompile(`^(#{1,6})(\s+(.*)|$)`)
)

// ProcessExtraction converts raw extraction results into a logical SemanticTree.
func (p *DefaultProcessor) ProcessExtraction(ctx context.Context, raw *model.RawExtractionResult) (*model.SemanticTree, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw extraction result cannot be nil")
	}

	p.mu.Lock()
	p.warnings = make([]string, 0)
	p.mu.Unlock()

	// Check if JSONLayout contains structured nodes
	if raw.JSONLayout != nil {
		if nodes, ok := raw.JSONLayout["nodes"].([]interface{}); ok && len(nodes) > 0 {
			return p.processJSONNodes(nodes)
		}
	}

	return p.processMarkdown(raw.Markdown)
}

func (p *DefaultProcessor) processMarkdown(markdown string) (*model.SemanticTree, error) {
	lines := strings.Split(markdown, "\n")
	currentPage := 1
	hasPreambleContent := false
	foundFirstHeading := false
	nodeCounter := 0

	var roots []*nodeBuilder
	var stack []*nodeBuilder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Page break & page marker detection
		if pageMatch := pageMarkerRegex.FindStringSubmatch(trimmed); pageMatch != nil {
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

		if pageBreakRegex.MatchString(trimmed) || strings.Contains(line, "\f") {
			currentPage++
			continue
		}

		if trimmed == "" {
			continue
		}

		// Heading detection
		if headMatch := headingRegex.FindStringSubmatch(trimmed); headMatch != nil {
			level := len(headMatch[1])
			headingText := strings.TrimSpace(headMatch[2])

			if headingText == "" {
				p.addWarning(fmt.Sprintf("empty heading text encountered at page %d", currentPage))
				headingText = "Untitled Section"
			}

			if !foundFirstHeading {
				if hasPreambleContent {
					p.addWarning(fmt.Sprintf("preamble text detected before first heading node at page %d", currentPage))
				}
				foundFirstHeading = true
			}

			nodeCounter++
			nodeID := fmt.Sprintf("sec-%d", nodeCounter)
			newNode := &nodeBuilder{
				ID:          nodeID,
				Heading:     headingText,
				Level:       level,
				PageNumbers: []int{currentPage},
				Children:    make([]*nodeBuilder, 0),
			}

			// Pop stack until parent level < newNode level
			for len(stack) > 0 && stack[len(stack)-1].Level >= level {
				stack = stack[:len(stack)-1]
			}

			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				if level > parent.Level+1 {
					p.addWarning(fmt.Sprintf("heading level skipped: expected level %d before level %d for heading '%s'", parent.Level+1, level, headingText))
				}
				parent.Children = append(parent.Children, newNode)
			} else {
				roots = append(roots, newNode)
			}

			stack = append(stack, newNode)
		} else {
			// Non-heading content
			if !foundFirstHeading {
				hasPreambleContent = true
			} else {
				// Mark currentPage on all ancestor nodes in stack
				for _, n := range stack {
					n.addPage(currentPage)
				}
			}
		}
	}

	// Fallback if no headings found but text exists
	if !foundFirstHeading && hasPreambleContent {
		p.addWarning(fmt.Sprintf("preamble text detected before first heading node at page %d", currentPage))
		nodeCounter++
		fallbackNode := &nodeBuilder{
			ID:          fmt.Sprintf("sec-%d", nodeCounter),
			Heading:     "Document Overview",
			Level:       1,
			PageNumbers: []int{1},
			Children:    make([]*nodeBuilder, 0),
		}
		roots = append(roots, fallbackNode)
	}

	resRoots := make([]model.SemanticNode, 0, len(roots))
	for _, r := range roots {
		resRoots = append(resRoots, convertBuilder(r))
	}

	tree := &model.SemanticTree{
		RootNodes: resRoots,
	}

	return tree, nil
}

func (p *DefaultProcessor) processJSONNodes(nodes []interface{}) (*model.SemanticTree, error) {
	var roots []*nodeBuilder
	var stack []*nodeBuilder
	nodeCounter := 0

	for _, item := range nodes {
		nodeMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		nodeType, _ := nodeMap["type"].(string)
		pg := 1
		if pgVal, ok := nodeMap["page"]; ok {
			switch v := pgVal.(type) {
			case float64:
				pg = int(v)
			case int:
				pg = v
			}
		}

		if pg < 1 {
			p.addWarning(fmt.Sprintf("ambiguous page reference %d in JSON layout node", pg))
			pg = 1
		}

		if nodeType == "heading" || nodeType == "header" {
			level := 1
			if lVal, ok := nodeMap["level"]; ok {
				switch v := lVal.(type) {
				case float64:
					level = int(v)
				case int:
					level = v
				}
			}
			text, _ := nodeMap["text"].(string)
			text = strings.TrimSpace(text)
			if text == "" {
				p.addWarning(fmt.Sprintf("empty heading text encountered in JSON layout at page %d", pg))
				text = "Untitled Section"
			}

			nodeCounter++
			newNode := &nodeBuilder{
				ID:          fmt.Sprintf("sec-%d", nodeCounter),
				Heading:     text,
				Level:       level,
				PageNumbers: []int{pg},
				Children:    make([]*nodeBuilder, 0),
			}

			for len(stack) > 0 && stack[len(stack)-1].Level >= level {
				stack = stack[:len(stack)-1]
			}

			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				if level > parent.Level+1 {
					p.addWarning(fmt.Sprintf("heading level skipped: expected level %d before level %d for heading '%s'", parent.Level+1, level, text))
				}
				parent.Children = append(parent.Children, newNode)
			} else {
				roots = append(roots, newNode)
			}

			stack = append(stack, newNode)
		} else if nodeType == "paragraph" || nodeType == "text" || nodeType == "table" || nodeType == "code" || nodeType == "list" {
			for _, n := range stack {
				n.addPage(pg)
			}
		} else {
			p.addWarning(fmt.Sprintf("unmapped custom layout node type: '%s' on page %d", nodeType, pg))
			for _, n := range stack {
				n.addPage(pg)
			}
		}
	}

	resRoots := make([]model.SemanticNode, 0, len(roots))
	for _, r := range roots {
		resRoots = append(resRoots, convertBuilder(r))
	}

	return &model.SemanticTree{RootNodes: resRoots}, nil
}

