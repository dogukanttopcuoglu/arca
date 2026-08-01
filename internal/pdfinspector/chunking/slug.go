package chunking

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	nonAlphaNumericRegex = regexp.MustCompile(`[^a-z0-9]+`)
)

// Slugify converts a section path into a deterministic, lowercased, URL-safe slug.
func Slugify(path string) string {
	if strings.TrimSpace(path) == "" {
		return "section"
	}

	// Normalize Unicode (NFD decompose, remove non-spacing marks)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, err := transform.String(t, path)
	if err != nil {
		normalized = path
	}

	lowered := strings.ToLower(normalized)
	slug := nonAlphaNumericRegex.ReplaceAllString(lowered, "-")
	slug = strings.Trim(slug, "-")

	if slug == "" {
		return "section"
	}
	return slug
}
