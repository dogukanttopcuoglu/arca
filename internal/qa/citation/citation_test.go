package citation_test

import (
	"testing"

	qacontext "arca/internal/qa/context"
	qacitation "arca/internal/qa/citation"
)

func TestCitationExtractor(t *testing.T) {
	extractor := qacitation.NewDefaultCitationExtractor()

	win := &qacontext.ContextWindow{
		Content: "[Ref 1] Source: doc-1 (Page 12)\nCreativity is a discipline.\n\n[Ref 2] Source: doc-1 (Page 18)\nFlow state requires focus.\n\n",
		Sources: []qacontext.SourceReference{
			{CitationKey: "[Ref 1]", DocumentID: "doc-1", ChunkID: "chk-1", SectionPath: "Intro", PageNumbers: []int{12}},
			{CitationKey: "[Ref 2]", DocumentID: "doc-1", ChunkID: "chk-2", SectionPath: "Flow", PageNumbers: []int{18}},
		},
		TokenCount: 30,
	}

	t.Run("extracts valid inline reference markers and maps to sources", func(t *testing.T) {
		llmOutput := "Creativity is a lifestyle [Ref 1]. Flow state is essential [Ref 2]."

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during citation extraction: %v", err)
		}

		if len(extracted) != 2 {
			t.Fatalf("expected 2 extracted citations, got %d", len(extracted))
		}
		if extracted[0].ChunkID != "chk-1" {
			t.Errorf("expected first citation chunk 'chk-1', got %s", extracted[0].ChunkID)
		}
		if extracted[1].ChunkID != "chk-2" {
			t.Errorf("expected second citation chunk 'chk-2', got %s", extracted[1].ChunkID)
		}

		if report.TotalClaims != 2 {
			t.Errorf("expected 2 total claims, got %d", report.TotalClaims)
		}
		if report.VerifiedClaims != 2 {
			t.Errorf("expected 2 verified claims, got %d", report.VerifiedClaims)
		}
		if report.InvalidReferences != 0 {
			t.Errorf("expected 0 invalid references, got %d", report.InvalidReferences)
		}
	})

	t.Run("flags hallucinated or invalid reference markers in report", func(t *testing.T) {
		llmOutput := "Creativity is nice [Ref 99]." // [Ref 99] does not exist in ContextWindow

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during extraction: %v", err)
		}

		if len(extracted) != 0 {
			t.Errorf("expected 0 valid extracted citations for invalid ref, got %d", len(extracted))
		}
		if report.InvalidReferences != 1 {
			t.Errorf("expected 1 invalid reference in report, got %d", report.InvalidReferences)
		}
	})
}
