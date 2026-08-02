package enrichment

import (
	"context"
	"sort"
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// ConceptInput holds targeted inputs for concept extraction.
type ConceptInput struct {
	Tree     *pdfmodel.SemanticTree
	Chunks   []pdfmodel.KnowledgeChunk
	Keywords []pdfmodel.Keyword
	Entities []pdfmodel.Entity
	Language string
}

// ConceptExtractor defines the strategy seam for extracting abstract domain concepts.
type ConceptExtractor interface {
	ExtractConcepts(ctx context.Context, input ConceptInput) ([]pdfmodel.Concept, error)
}

// RuleBasedConceptExtractor implements ConceptExtractor by synthesizing headings and keywords deterministically.
type RuleBasedConceptExtractor struct{}

// NewRuleBasedConceptExtractor constructs a RuleBasedConceptExtractor instance.
func NewRuleBasedConceptExtractor() *RuleBasedConceptExtractor {
	return &RuleBasedConceptExtractor{}
}

// ExtractConcepts discovers abstract concepts from section headings and key phrases.
func (e *RuleBasedConceptExtractor) ExtractConcepts(ctx context.Context, input ConceptInput) ([]pdfmodel.Concept, error) {
	seen := make(map[string]bool)
	var concepts []pdfmodel.Concept

	// 1. Synthesize concepts from H1-H3 section headings in SemanticTree
	if input.Tree != nil {
		var extractFromNode func(n pdfmodel.SemanticNode)
		extractFromNode = func(n pdfmodel.SemanticNode) {
			heading := strings.TrimSpace(n.Heading)
			if heading != "" && n.Level <= 3 {
				key := strings.ToLower(heading)
				if !seen[key] {
					seen[key] = true
					score := 0.90
					if n.Level == 1 {
						score = 0.98
					} else if n.Level == 2 {
						score = 0.94
					}
					concepts = append(concepts, pdfmodel.Concept{
						ID:     "concept:" + slugify(heading),
						Name:   heading,
						Score:  score,
						Source: pdfmodel.ConceptSourceRuleBased,
					})
				}
			}
			for _, child := range n.Children {
				extractFromNode(child)
			}
		}

		for _, root := range input.Tree.RootNodes {
			extractFromNode(root)
		}
	}

	// Build entity fragment lookup map to prevent entity fragments (e.g. "york", "def") from leaking into concepts
	entityFragments := make(map[string]bool)
	for _, ent := range input.Entities {
		words := strings.Fields(strings.ToLower(ent.Name))
		for _, w := range words {
			if len(w) > 2 {
				entityFragments[w] = true
			}
		}
	}

	// 2. Synthesize concepts from top high-salience multi-word Keywords (score >= 0.75)
	for _, kw := range input.Keywords {
		trimmedKw := strings.TrimSpace(kw.Value)
		key := strings.ToLower(trimmedKw)

		// Concept Domain Boundary Rule: Concepts MUST NOT be single-word unigrams or entity fragments
		if kw.Score >= 0.75 && len(trimmedKw) > 3 && strings.Contains(trimmedKw, " ") && !entityFragments[key] {
			if !seen[key] {
				seen[key] = true
				concepts = append(concepts, pdfmodel.Concept{
					ID:     "concept:" + slugify(trimmedKw),
					Name:   trimmedKw,
					Score:  kw.Score,
					Source: pdfmodel.ConceptSourceRuleBased,
				})
			}
		}
	}

	// Sort concepts by Score descending for deterministic output ranking
	sort.Slice(concepts, func(i, j int) bool {
		if concepts[i].Score == concepts[j].Score {
			return concepts[i].Name < concepts[j].Name
		}
		return concepts[i].Score > concepts[j].Score
	})

	return concepts, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
