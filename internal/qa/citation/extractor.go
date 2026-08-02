package citation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	qacontext "arca/internal/qa/context"
)

// AnswerCitation models a verified citation attached to a RAG answer.
type AnswerCitation struct {
	CitationKey string `json:"citation_key"`
	DocumentID  string `json:"document_id"`
	ChunkID     string `json:"chunk_id"`
	SectionPath string `json:"section_path,omitempty"`
	PageNumbers []int  `json:"page_numbers,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
}

// VerificationReport captures structural citation metrics and validity counts.
type VerificationReport struct {
	TotalClaims       int `json:"total_claims"`
	VerifiedClaims    int `json:"verified_claims"`
	MissingCitations  int `json:"missing_citations"`
	InvalidReferences int `json:"invalid_references"`
}

// CitationExtractor defines the seam for extracting and validating inline markers from LLM output.
type CitationExtractor interface {
	Extract(answerText string, win *qacontext.ContextWindow) ([]AnswerCitation, VerificationReport, error)
}

// DefaultCitationExtractor implements CitationExtractor using regex pattern matching and immutable source mapping.
type DefaultCitationExtractor struct{}

// NewDefaultCitationExtractor constructs a DefaultCitationExtractor instance.
func NewDefaultCitationExtractor() *DefaultCitationExtractor {
	return &DefaultCitationExtractor{}
}

var refRegex = regexp.MustCompile(`\[Ref\s+(\d+)\]`)

// Extract parses inline reference markers (`[Ref N]`) and verifies them against ContextWindow sources.
func (e *DefaultCitationExtractor) Extract(answerText string, win *qacontext.ContextWindow) ([]AnswerCitation, VerificationReport, error) {
	if strings.TrimSpace(answerText) == "" {
		return nil, VerificationReport{}, fmt.Errorf("answer text cannot be empty")
	}

	report := VerificationReport{}
	if win == nil || len(win.Sources) == 0 {
		report.MissingCitations = 1
		return []AnswerCitation{}, report, nil
	}

	sourceMap := make(map[string]qacontext.SourceReference)
	for _, src := range win.Sources {
		sourceMap[src.CitationKey] = src
	}

	matches := refRegex.FindAllStringSubmatch(answerText, -1)
	report.TotalClaims = len(matches)

	seenKeys := make(map[string]bool)
	var citations []AnswerCitation

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		refNum, _ := strconv.Atoi(match[1])
		key := fmt.Sprintf("[Ref %d]", refNum)

		src, exists := sourceMap[key]
		if !exists {
			report.InvalidReferences++
			continue
		}

		if !seenKeys[key] {
			seenKeys[key] = true
			report.VerifiedClaims++
			citations = append(citations, AnswerCitation{
				CitationKey: key,
				DocumentID:  src.DocumentID,
				ChunkID:     src.ChunkID,
				SectionPath: src.SectionPath,
				PageNumbers: src.PageNumbers,
				Snippet:     src.Content,
			})
		}
	}

	return citations, report, nil
}
