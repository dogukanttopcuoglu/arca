package citation_test

import (
	"testing"

	qacitation "arca/internal/qa/citation"
	qacontext "arca/internal/qa/context"
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

	t.Run("extracts combined markers in a single bracket", func(t *testing.T) {
		llmOutput := "Creativity is a lifestyle and flow is essential [Ref 1, 2]."

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during citation extraction: %v", err)
		}

		if len(extracted) != 2 {
			t.Fatalf("expected 2 extracted citations from [Ref 1, 2], got %d", len(extracted))
		}
		if extracted[0].ChunkID != "chk-1" || extracted[1].ChunkID != "chk-2" {
			t.Errorf("expected citations for chk-1 and chk-2, got %+v", extracted)
		}
		if report.TotalClaims != 2 || report.VerifiedClaims != 2 || report.InvalidReferences != 0 {
			t.Errorf("unexpected report for combined markers: %+v", report)
		}
	})

	t.Run("extracts repeated Ref markers in a single bracket", func(t *testing.T) {
		llmOutput := "Both claims are supported [Ref 1, Ref 2]."

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during citation extraction: %v", err)
		}

		if len(extracted) != 2 {
			t.Fatalf("expected 2 extracted citations from [Ref 1, Ref 2], got %d", len(extracted))
		}
		if report.VerifiedClaims != 2 {
			t.Errorf("expected 2 verified claims, got %d", report.VerifiedClaims)
		}
	})

	t.Run("handles mixed single and combined markers", func(t *testing.T) {
		llmOutput := "One claim [Ref 1] and another [Ref 2, 1]."

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during citation extraction: %v", err)
		}

		if len(extracted) != 2 {
			t.Fatalf("expected 2 unique citations, got %d", len(extracted))
		}
		if report.TotalClaims != 3 || report.VerifiedClaims != 2 {
			t.Errorf("expected 3 claims with 2 verified (deduped), got %+v", report)
		}
	})

	t.Run("flags invalid references inside combined markers", func(t *testing.T) {
		llmOutput := "Speculative claims [Ref 9, 10]."

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during citation extraction: %v", err)
		}

		if len(extracted) != 0 {
			t.Errorf("expected no valid citations, got %d", len(extracted))
		}
		if report.InvalidReferences != 2 {
			t.Errorf("expected 2 invalid references, got %d", report.InvalidReferences)
		}
	})

	t.Run("counts valid and invalid references inside one combined marker", func(t *testing.T) {
		llmOutput := "Partly supported claims [Ref 1, 99]."

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during citation extraction: %v", err)
		}

		if len(extracted) != 1 || extracted[0].ChunkID != "chk-1" {
			t.Errorf("expected exactly the valid citation, got %+v", extracted)
		}
		if report.VerifiedClaims != 1 || report.InvalidReferences != 1 {
			t.Errorf("expected 1 verified and 1 invalid, got %+v", report)
		}
	})

	t.Run("never treats bare number lists in prose as citations", func(t *testing.T) {
		llmOutput := "The inputs were [1, 2] as shown."

		extracted, report, err := extractor.Extract(llmOutput, win)
		if err != nil {
			t.Fatalf("unexpected error during citation extraction: %v", err)
		}

		if len(extracted) != 0 {
			t.Errorf("expected no citations from bare number list, got %d", len(extracted))
		}
		if report.TotalClaims != 0 {
			t.Errorf("expected 0 total claims, got %d", report.TotalClaims)
		}
	})
}
