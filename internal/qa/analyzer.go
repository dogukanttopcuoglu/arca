package qa

import (
	"context"
	"fmt"
	"strings"

	indexingmodel "arca/internal/indexing/model"
)

// AnalyzedQuery represents structured intent, entities, and metadata filters extracted from natural query text.
type AnalyzedQuery struct {
	RawQuery      string                       `json:"raw_query"`
	Intent        string                       `json:"intent"`
	Entities      []string                     `json:"entities,omitempty"`
	ExtractedFilter indexingmodel.MetadataFilter `json:"extracted_filter,omitempty"`
}

// QueryAnalyzer defines the domain interface seam for query understanding and intent analysis.
type QueryAnalyzer interface {
	// Analyze parses natural language text into an AnalyzedQuery.
	Analyze(ctx context.Context, query string) (*AnalyzedQuery, error)
}

// RuleBasedAnalyzer is a fast, rule-based default adapter for QueryAnalyzer.
type RuleBasedAnalyzer struct{}

// NewRuleBasedAnalyzer constructs a RuleBasedAnalyzer instance.
func NewRuleBasedAnalyzer() *RuleBasedAnalyzer {
	return &RuleBasedAnalyzer{}
}

// Analyze extracts basic query intent and keywords.
func (a *RuleBasedAnalyzer) Analyze(ctx context.Context, query string) (*AnalyzedQuery, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("query string cannot be empty")
	}

	intent := "concept_lookup"
	if strings.HasPrefix(strings.ToLower(trimmed), "who") || strings.Contains(strings.ToLower(trimmed), "author") {
		intent = "entity_lookup"
	} else if strings.HasPrefix(strings.ToLower(trimmed), "how") {
		intent = "procedural_lookup"
	}

	// Extract simple terms
	words := strings.Fields(trimmed)
	entities := make([]string, 0, len(words))
	for _, w := range words {
		cleaned := strings.Trim(w, "?!.,\"'")
		if len(cleaned) > 3 {
			entities = append(entities, cleaned)
		}
	}

	return &AnalyzedQuery{
		RawQuery: trimmed,
		Intent:   intent,
		Entities: entities,
	}, nil
}
