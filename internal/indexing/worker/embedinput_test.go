package worker

import (
	"strings"
	"testing"

	pdfmodel "arca/internal/pdfinspector/model"
)

func chunkWithPath(path string) pdfmodel.KnowledgeChunk {
	return pdfmodel.KnowledgeChunk{
		ChunkID:         "doc/tuning-in/002",
		DocumentID:      "doc",
		SectionPath:     path,
		ContentMarkdown: "Think of the universe as an eternal creative unfolding.",
	}
}

func TestBuildEmbeddingInputContentOnly(t *testing.T) {
	in := BuildEmbeddingInput(chunkWithPath("Creativity > Tuning In"), "The Creative Act", RepresentationContent)
	if in != "Think of the universe as an eternal creative unfolding." {
		t.Fatalf("content-only representation changed the input: %q", in)
	}
}

func TestBuildEmbeddingInputSectionTitle(t *testing.T) {
	in := BuildEmbeddingInput(chunkWithPath("Creativity > Tuning In"), "The Creative Act", RepresentationSectionTitle)
	if !strings.HasPrefix(in, "Tuning In\n\n") {
		t.Fatalf("section title not prepended: %q", in)
	}
	if !strings.Contains(in, "Think of the universe") {
		t.Fatalf("body missing: %q", in)
	}
}

func TestBuildEmbeddingInputSectionPath(t *testing.T) {
	in := BuildEmbeddingInput(chunkWithPath("Creativity > Tuning In"), "The Creative Act", RepresentationSectionPath)
	if !strings.HasPrefix(in, "Creativity > Tuning In\n\n") {
		t.Fatalf("section path not prepended: %q", in)
	}
}

func TestBuildEmbeddingInputBookPath(t *testing.T) {
	in := BuildEmbeddingInput(chunkWithPath("Creativity > Tuning In"), "The Creative Act", RepresentationBookPath)
	if !strings.HasPrefix(in, "The Creative Act > Creativity > Tuning In\n\n") {
		t.Fatalf("book path not prepended: %q", in)
	}
}

func TestBuildEmbeddingInputBookPathFallsBackToDocumentID(t *testing.T) {
	// Empty (or unresolvable) document titles fall back to the document ID
	// slug — deterministic, quality-independent (ADR-0047).
	in := BuildEmbeddingInput(chunkWithPath("Tuning In"), "  ", RepresentationBookPath)
	if !strings.HasPrefix(in, "doc > Tuning In\n\n") {
		t.Fatalf("document-id fallback not applied: %q", in)
	}
}

func TestBuildEmbeddingInputEmptyPath(t *testing.T) {
	in := BuildEmbeddingInput(chunkWithPath(""), "The Creative Act", RepresentationBookPath)
	if in != "The Creative Act\n\nThink of the universe as an eternal creative unfolding." {
		t.Fatalf("empty path handling wrong: %q", in)
	}
}
