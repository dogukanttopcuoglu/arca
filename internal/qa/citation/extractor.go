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

// Claim is a sentence extracted from an answer that carries at least one
// inline reference marker.
type Claim struct {
	Sentence string
	Refs     []int
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

// combinedRefRegex matches a single bracket holding a comma-separated
// reference list, as LLMs commonly emit: "[Ref 1, 2]" or "[Ref 1, Ref 2]".
// The first element must be Ref-prefixed so bare number lists in prose
// (e.g. "[1, 2]") are never treated as citations.
var combinedRefRegex = regexp.MustCompile(`\[(Ref\s+\d+(?:\s*,\s*(?:Ref\s+)?\d+)+)\]`)

// ExtractClaims splits answerText into sentences and keeps only the ones
// that carry at least one reference marker. Combined markers are expanded
// through the same normalization Extract uses, so a claim's Refs cover
// every reference in a "[Ref 1, 2]" bracket. Newlines and terminator
// characters (followed by a space, closing quote, or end of string) are
// sentence boundaries; markdown cosmetics (bold markers, heading hashes,
// list bullets) are stripped from the kept sentence. Sentences without
// markers and empty text produce nil.
func ExtractClaims(answerText string) []Claim {
	if strings.TrimSpace(answerText) == "" {
		return nil
	}
	normalized := normalizeCombinedMarkers(answerText)
	var claims []Claim
	for _, sentence := range splitSentences(normalized) {
		matches := refRegex.FindAllStringSubmatch(sentence, -1)
		if len(matches) == 0 {
			continue
		}
		refs := make([]int, 0, len(matches))
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			refs = append(refs, n)
		}
		claims = append(claims, Claim{Sentence: cleanClaimText(sentence), Refs: refs})
	}
	return claims
}

// cleanClaimText strips markdown cosmetics that carry no meaning for an
// entailment check: bold markers, heading hashes, and leading list bullets.
func cleanClaimText(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "#")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "*") {
		s = strings.TrimSpace(s[1:])
	}
	return s
}

// splitSentences cuts text on newlines and on terminator characters that
// fall outside quoted spans, followed by a space, a closing quote, or the
// end of the string. Terminators inside quotes ("...more energy. It's...")
// and inside prose ("e.g.", decimals) do not split, so quoted spans stay
// whole. Trailing whitespace is trimmed and empty sentences dropped.
func splitSentences(text string) []string {
	runes := []rune(text)
	var sentences []string
	start := 0
	inQuote := false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '"', '“', '”':
			inQuote = !inQuote
			continue
		case '\n':
			if inQuote {
				continue // quoted span continues across the line break
			}
			if s := strings.TrimSpace(string(runes[start:i])); s != "" {
				sentences = append(sentences, s)
			}
			start = i + 1
			continue
		case '.', '!', '?':
			if inQuote {
				continue
			}
			after := i + 1
			for after < len(runes) && (runes[after] == '”' || runes[after] == '"') {
				after++
			}
			if after < len(runes) && runes[after] != ' ' {
				continue
			}
			if s := strings.TrimSpace(string(runes[start:after])); s != "" {
				sentences = append(sentences, s)
			}
			start = after
		}
	}
	if s := strings.TrimSpace(string(runes[start:])); s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}

// normalizeCombinedMarkers expands comma-separated reference lists into
// individual markers ("[Ref 1, 2]" -> "[Ref 1] [Ref 2]") so the standard
// marker regex can extract every reference. Single markers pass through
// unchanged. The original answer text is never rewritten — normalization is
// internal to extraction.
func normalizeCombinedMarkers(answerText string) string {
	return combinedRefRegex.ReplaceAllStringFunc(answerText, func(match string) string {
		inner := combinedRefRegex.FindStringSubmatch(match)[1]
		parts := strings.Split(inner, ",")
		markers := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.TrimPrefix(p, "Ref ")
			markers = append(markers, "[Ref "+p+"]")
		}
		return strings.Join(markers, " ")
	})
}

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

	normalized := normalizeCombinedMarkers(answerText)
	matches := refRegex.FindAllStringSubmatch(normalized, -1)
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
