package verification_test

import (
	"context"
	"testing"

	qacontext "arca/internal/qa/context"
	qaverification "arca/internal/qa/verification"
)

func TestVerificationPipeline(t *testing.T) {
	ctx := context.Background()
	pipeline := qaverification.NewDefaultVerificationPipeline()

	win := &qacontext.ContextWindow{
		Sources: []qacontext.SourceReference{
			{CitationKey: "[Ref 1]", DocumentID: "doc-1", ChunkID: "chk-1", SectionPath: "Intro", PageNumbers: []int{12}},
		},
	}

	t.Run("verifies valid draft text using structural verifier", func(t *testing.T) {
		text := "Discipline is key to creative act [Ref 1]."
		ans, err := pipeline.Verify(ctx, text, win)
		if err != nil {
			t.Fatalf("unexpected error during verification: %v", err)
		}

		if ans == nil {
			t.Fatal("expected non-nil VerifiedAnswer")
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("expected Status %q, got %q", qaverification.StatusVerified, ans.Status)
		}
		if len(ans.Citations) != 1 {
			t.Fatalf("expected 1 citation, got %d", len(ans.Citations))
		}
	})

	t.Run("marks answer unverified when invalid references are present", func(t *testing.T) {
		text := "Invalid claim without valid source [Ref 999]."
		ans, err := pipeline.Verify(ctx, text, win)
		if err != nil {
			t.Fatalf("unexpected error during verification: %v", err)
		}

		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("expected Status %q, got %q", qaverification.StatusUnverified, ans.Status)
		}
	})

	t.Run("marks answer unverified when answer cites no sources", func(t *testing.T) {
		text := "A claim with no citation markers at all."
		ans, err := pipeline.Verify(ctx, text, win)
		if err != nil {
			t.Fatalf("unexpected error during verification: %v", err)
		}

		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("expected Status %q, got %q", qaverification.StatusUnverified, ans.Status)
		}
	})
}
