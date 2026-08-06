package worker

import (
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// EmbeddingInputRepresentation selects how the embedding input text is built
// (ADR-0047). The representation is part of the IndexSignature contract via
// EmbeddingVersion: changing the production representation requires a
// version bump and a full re-index — the diff engine would otherwise see
// identical signatures with different vectors and keep stale points.
type EmbeddingInputRepresentation int

const (
	// RepresentationContent embeds only the chunk body (pre-ADR-0047
	// behavior — the production default until the probe accepts).
	RepresentationContent EmbeddingInputRepresentation = iota
	// RepresentationSectionTitle prepends the section title to the body.
	RepresentationSectionTitle
	// RepresentationSectionPath prepends the full section path.
	RepresentationSectionPath
	// RepresentationBookPath prepends "book title > section path" (the
	// ADR-0047 target): the book-level prefix disambiguates generic section
	// titles across the multi-document corpus.
	RepresentationBookPath
)

// BuildEmbeddingInput constructs the embedding input text for a chunk under
// the given representation. The same text feeds both dense and sparse
// encoding, so both representations stay consistent (worker.go).
func BuildEmbeddingInput(chunk pdfmodel.KnowledgeChunk, documentTitle string, rep EmbeddingInputRepresentation) string {
	switch rep {
	case RepresentationSectionTitle:
		return joinHeadings(lastSegment(chunk.SectionPath), chunk.ContentMarkdown)
	case RepresentationSectionPath:
		return joinHeadings(chunk.SectionPath, chunk.ContentMarkdown)
	case RepresentationBookPath:
		book := strings.TrimSpace(documentTitle)
		if book == "" {
			// Deterministic fallback: the document ID slug disambiguates
			// even when title resolution failed (TitleResolver fallbacks).
			book = chunk.DocumentID
		}
		prefix := book
		if chunk.SectionPath != "" {
			prefix = book + " > " + chunk.SectionPath
		}
		return joinHeadings(prefix, chunk.ContentMarkdown)
	default:
		return chunk.ContentMarkdown
	}
}

// lastSegment returns the final path segment ("Tuning In" for
// "Creativity > Tuning In").
func lastSegment(path string) string {
	if i := strings.LastIndex(path, " > "); i >= 0 {
		return path[i+3:]
	}
	return path
}

func joinHeadings(prefix, content string) string {
	if prefix == "" {
		return content
	}
	return prefix + "\n\n" + content
}
