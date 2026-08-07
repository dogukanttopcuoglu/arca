package qa

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	indexingmodel "arca/internal/indexing/model"
)

// AnalyzedQuery represents structured intent, entities, and metadata filters extracted from natural query text.
type AnalyzedQuery struct {
	RawQuery        string                       `json:"raw_query"`
	Intent          string                       `json:"intent"`
	Entities        []string                     `json:"entities,omitempty"`
	ExtractedFilter indexingmodel.MetadataFilter `json:"extracted_filter,omitempty"`
	// SubQueries holds deterministic decomposed sub-queries (e.g. the two
	// sides of a comparison). Nil for single-intent queries.
	SubQueries []string `json:"sub_queries,omitempty"`
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

// entityQuestionPattern matches the benchmark-proven entity question forms
// (M7 gold set v3 entity slice): "What does the book say about X?".
var entityQuestionPattern = regexp.MustCompile(`(?i)^what does the (?:book|it) say about .+\??$`)

// documentOverviewPatterns detect document-level questions (M9, ADR-0048):
// "what is this book about", "bu kitap ne anlatıyor", "yazar kim", "özetle".
// The set is deliberately capped — it covers the benchmarked forms; a 5th
// non-structural TR pattern request signals the exemplar-based matcher
// (Phase 2), never unbounded regex growth. Section-scoped summary queries
// (bölüm/chapter/konu + özet/summary) are excluded before matching.
var documentOverviewPatterns = []*regexp.Regexp{
	// EN — structural, book-anchored
	regexp.MustCompile(`(?i)^what is (this|the) book about`),
	regexp.MustCompile(`(?i)^what (does|do) (this|the) book cover`),
	regexp.MustCompile(`(?i)^summar(ize|y) (of )?(this|the) book`),
	regexp.MustCompile(`(?i)^what is the main idea`),
	regexp.MustCompile(`(?i)^who (is|was) the author`),
	regexp.MustCompile(`(?i)^who wrote (this|the) book`),
	// TR — capped, book/author-anchored
	regexp.MustCompile(`(?i)kitap ne anlatıyor`),
	regexp.MustCompile(`(?i)kitap ne hakkında`),
	regexp.MustCompile(`(?i)kitabı özetle`),
	regexp.MustCompile(`(?i)^özetle`),
	regexp.MustCompile(`(?i)ana fikri`),
	regexp.MustCompile(`(?i)^yazar kim`),
	regexp.MustCompile(`(?i)^yazarı kim`),
	regexp.MustCompile(`(?i)kim yazdı`),
}

// isSectionScopedSummary reports whether the query asks for a section-level
// summary (bölüm/chapter/section/konu/topic + özet/summary/summarize) — such
// queries must stay on normal retrieval, never route to document overview.
func isSectionScopedSummary(trimmed string) bool {
	low := strings.ToLower(trimmed)
	sectionWord := strings.Contains(low, "bölüm") || strings.Contains(low, "chapter") ||
		strings.Contains(low, "section") || strings.Contains(low, "konu") || strings.Contains(low, "topic")
	summaryWord := strings.Contains(low, "özet") || strings.Contains(low, "summary") || strings.Contains(low, "summarize")
	return sectionWord && summaryWord
}

// Analyze extracts basic query intent, keywords, and deterministic
// sub-queries for comparison patterns (M4 decomposition experiment). Entity
// question forms are flagged for the M7 graph gate (ADR-0042); document
// overview forms for the M9 document-level path (ADR-0048).
func (a *RuleBasedAnalyzer) Analyze(ctx context.Context, query string) (*AnalyzedQuery, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("query string cannot be empty")
	}

	intent := "concept_lookup"
	switch {
	case entityQuestionPattern.MatchString(trimmed):
		intent = "entity_lookup"
	case !isSectionScopedSummary(trimmed) && matchesAny(documentOverviewPatterns, trimmed):
		intent = "document_overview"
	case strings.HasPrefix(strings.ToLower(trimmed), "who") || strings.Contains(strings.ToLower(trimmed), "author"):
		intent = "entity_lookup"
	case strings.HasPrefix(strings.ToLower(trimmed), "how"):
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
		RawQuery:   trimmed,
		Intent:     intent,
		Entities:   entities,
		SubQueries: decomposeComparison(trimmed),
	}, nil
}

// matchesAny reports whether the query matches any of the patterns.
func matchesAny(patterns []*regexp.Regexp, query string) bool {
	for _, re := range patterns {
		if re.MatchString(query) {
			return true
		}
	}
	return false
}

// decomposeComparison deterministically splits comparison queries into two
// sub-queries using rule-based patterns. Returns nil when no pattern matches.
func decomposeComparison(query string) []string {
	patterns := []*regexp.Regexp{
		// "Compare X with Y" / "Compare X and Y"
		regexp.MustCompile(`(?i)^compare\s+(.+?)\s+(?:with|and)\s+(.+)$`),
		// "X vs Y"
		regexp.MustCompile(`(?i)^(.+?)\s+vs\.?\s+(.+)$`),
		// "Explain the difference between X and Y"
		regexp.MustCompile(`(?i)^.*?\bdifference\s+between\s+(.+?)\s+and\s+(.+)$`),
		// "How do X and Y differ?"
		regexp.MustCompile(`(?i)^how\s+do\s+(.+?)\s+and\s+(.+?)\s+differ`),
		// "Contrast the approaches of X and Y" -> strip the lead-in
		regexp.MustCompile(`(?i)^contrast\s+(?:the\s+approaches\s+of\s+)?(.+?)\s+and\s+(.+)$`),
		// "What distinguishes X from Y?"
		regexp.MustCompile(`(?i)^what\s+distinguishes\s+(.+?)\s+from\s+(.+)$`),
	}

	for _, re := range patterns {
		if m := re.FindStringSubmatch(query); m != nil && len(m) == 3 {
			left := strings.TrimSpace(m[1])
			right := strings.TrimSpace(m[2])
			if left == "" || right == "" || strings.EqualFold(left, right) {
				continue
			}
			return []string{left, right}
		}
	}
	return nil
}
