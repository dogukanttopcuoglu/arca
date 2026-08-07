package worker

import (
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// ProfileSectionPath is the section path of the Document Profile Point
// (ADR-0048). It is a document-level retrieval artifact marker, never a
// corpus section.
const ProfileSectionPath = "Document Profile"

// maxProfileFields caps the deterministic profile fields so the content
// stays bounded and stable for arbitrary documents.
const (
	maxProfileKeywords = 8
	maxProfileSections = 8
)

// BuildDocumentProfileContent renders the deterministic profile text from
// enrichment metadata (ADR-0048): title, author, extractive summary, key
// topics, and the document's section list. The output is stable for
// identical metadata — its ContentHash drives the diff lifecycle.
func BuildDocumentProfileContent(meta *pdfmodel.DocumentMetadata, chunks []pdfmodel.KnowledgeChunk) string {
	var sb strings.Builder
	if strings.TrimSpace(meta.Title) != "" {
		sb.WriteString("Title: " + strings.TrimSpace(meta.Title) + "\n")
	}
	if strings.TrimSpace(meta.Author) != "" {
		sb.WriteString("Author: " + strings.TrimSpace(meta.Author) + "\n")
	}
	if meta.Summary != nil && strings.TrimSpace(meta.Summary.Text) != "" {
		sb.WriteString("Summary: " + strings.TrimSpace(meta.Summary.Text) + "\n")
	}
	if len(meta.Keywords) > 0 {
		var vals []string
		for _, k := range meta.Keywords {
			if v := strings.TrimSpace(k.Value); v != "" {
				vals = append(vals, v)
			}
			if len(vals) >= maxProfileKeywords {
				break
			}
		}
		if len(vals) > 0 {
			sb.WriteString("Key Topics: " + strings.Join(vals, ", ") + "\n")
		}
	}
	if sections := uniqueSectionPaths(chunks); len(sections) > 0 {
		sb.WriteString("Sections: " + strings.Join(sections, ", ") + "\n")
	}
	return strings.TrimSpace(sb.String())
}

// uniqueSectionPaths returns the distinct section paths in first-seen order,
// capped at maxProfileSections.
func uniqueSectionPaths(chunks []pdfmodel.KnowledgeChunk) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, maxProfileSections)
	for _, ch := range chunks {
		p := strings.TrimSpace(ch.SectionPath)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) >= maxProfileSections {
			break
		}
	}
	return out
}
