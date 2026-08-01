package chunking

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var (
	multiBlankLineRegex = regexp.MustCompile(`\n{3,}`)
)

// NormalizeMarkdown cleans up whitespace, normalizes line endings (\r\n -> \n), trims trailing spaces on lines, and collapses extra blank lines.
func NormalizeMarkdown(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	joined := strings.Join(lines, "\n")
	collapsed := multiBlankLineRegex.ReplaceAllString(joined, "\n\n")
	return strings.TrimSpace(collapsed)
}

// ComputeContentHash returns the SHA-256 hex string of normalized Markdown.
func ComputeContentHash(markdown string) string {
	norm := NormalizeMarkdown(markdown)
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// ComputeFingerprint returns the SHA-256 hex string of document identity and structural location combined with normalized Markdown.
func ComputeFingerprint(docID, sectionPath string, headingLevel int, markdown string, pages []int) string {
	norm := NormalizeMarkdown(markdown)
	payload := fmt.Sprintf("%s|%s|%d|%v|%s", docID, sectionPath, headingLevel, pages, norm)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
