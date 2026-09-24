package verification_test

import (
	"context"
	"testing"

	qacontext "arca/internal/qa/context"
	qaverification "arca/internal/qa/verification"
)

// fakeChecker records every (claim, source) pair and returns scripted
// verdicts. sourceResults win over results when both key a pair.
type fakeChecker struct {
	results       map[string]qaverification.EntailmentScore
	sourceResults map[string]qaverification.EntailmentScore
	err           error
	calls         int
	claims        []string
	sources       []string
}

func (f *fakeChecker) CheckEntailment(ctx context.Context, claim, sourceText string) (qaverification.EntailmentScore, error) {
	f.calls++
	f.claims = append(f.claims, claim)
	f.sources = append(f.sources, sourceText)
	if f.err != nil {
		return qaverification.EntailmentScore{}, f.err
	}
	if score, ok := f.sourceResults[sourceText]; ok {
		return score, nil
	}
	if score, ok := f.results[claim]; ok {
		return score, nil
	}
	return qaverification.EntailmentScore{Score: 1, Relation: "entailed"}, nil
}

func TestPipelinePhase2(t *testing.T) {
	ctx := context.Background()
	win := &qacontext.ContextWindow{
		Sources: []qacontext.SourceReference{
			{CitationKey: "[Ref 1]", DocumentID: "doc-1", Content: "source content one"},
			{CitationKey: "[Ref 2]", DocumentID: "doc-2", Content: "source content two"},
		},
	}

	entailed := func(claim string) *fakeChecker {
		return &fakeChecker{results: map[string]qaverification.EntailmentScore{
			claim: {Score: 0.95, Relation: "entailed"},
		}}
	}

	t.Run("all entailed keeps Phase 1 verified", func(t *testing.T) {
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(entailed("Grounded claim [Ref 1]."))

		ans, err := pipeline.Verify(ctx, "Grounded claim [Ref 1].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("status = %q, want verified", ans.Status)
		}
		if ans.Degraded {
			t.Error("degraded must stay false on success")
		}
		if len(ans.Semantic) != 1 || ans.Semantic[0].Relation != "entailed" {
			t.Errorf("semantic = %+v, want one entailed verdict", ans.Semantic)
		}
		if ans.Semantic[0].Score != 0.95 {
			t.Errorf("score = %v, want 0.95", ans.Semantic[0].Score)
		}
	})

	t.Run("one contradicted verdict downgrades to unverified", func(t *testing.T) {
		checker := &fakeChecker{results: map[string]qaverification.EntailmentScore{
			"Contradicted claim [Ref 1].": {Score: 0.9, Relation: "contradicted"},
		}}
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "Contradicted claim [Ref 1].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("status = %q, want unverified", ans.Status)
		}
		if ans.Degraded {
			t.Error("contradiction is a successful check, not a degradation")
		}
		if len(ans.Semantic) != 1 || ans.Semantic[0].Relation != "contradicted" {
			t.Errorf("semantic = %+v, want one contradicted verdict", ans.Semantic)
		}
	})

	t.Run("checker error fails open with Phase 1 status preserved", func(t *testing.T) {
		checker := &fakeChecker{err: context.DeadlineExceeded}
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "Grounded claim [Ref 1].", win)
		if err != nil {
			t.Fatalf("Verify must not propagate a Phase 2 failure, got %v", err)
		}
		if !ans.Degraded {
			t.Error("degraded must be set on checker failure")
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("status = %q, want Phase 1 verified preserved", ans.Status)
		}
		if ans.Semantic != nil {
			t.Errorf("semantic = %+v, want nil on fail-open", ans.Semantic)
		}
	})

	t.Run("nil checker preserves existing behavior", func(t *testing.T) {
		pipeline := qaverification.NewDefaultVerificationPipeline()

		ans, err := pipeline.Verify(ctx, "Grounded claim [Ref 1].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("status = %q, want verified", ans.Status)
		}
		if ans.Degraded {
			t.Error("degraded must stay false with no checker")
		}
		if ans.Semantic != nil {
			t.Errorf("semantic = %+v, want nil with no checker", ans.Semantic)
		}
	})

	t.Run("refs invalid in Phase 1 are skipped in Phase 2", func(t *testing.T) {
		checker := entailed("Good [Ref 1].")
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "Good [Ref 1]. Bad [Ref 99].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("status = %q, want unverified (invalid ref in Phase 1)", ans.Status)
		}
		if checker.calls != 1 {
			t.Errorf("checker calls = %d, want 1 (only the valid ref)", checker.calls)
		}
		if checker.claims[0] != "Good [Ref 1]." {
			t.Errorf("checked claim = %q", checker.claims[0])
		}
		if len(ans.Semantic) != 1 {
			t.Errorf("semantic = %+v, want only the valid pair", ans.Semantic)
		}
	})

	t.Run("multiple refs on one claim check each pair and pass the source content", func(t *testing.T) {
		checker := entailed("Both [Ref 1] [Ref 2].")
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "Both [Ref 1, 2].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("status = %q, want verified", ans.Status)
		}
		if checker.calls != 2 {
			t.Errorf("checker calls = %d, want 2", checker.calls)
		}
		if len(ans.Semantic) != 2 {
			t.Fatalf("semantic = %+v, want 2 verdicts", ans.Semantic)
		}
		if checker.sources[0] != "source content one" || checker.sources[1] != "source content two" {
			t.Errorf("sources passed = %v", checker.sources)
		}
	})

	t.Run("no refs anywhere skips Phase 2 entirely", func(t *testing.T) {
		checker := entailed("no claim here")
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "A claim with no citations.", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if checker.calls != 0 {
			t.Errorf("checker calls = %d, want 0", checker.calls)
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("status = %q, want unverified", ans.Status)
		}
	})
}

func TestPipelinePhase2_ClaimLevelSupport(t *testing.T) {
	ctx := context.Background()
	win := &qacontext.ContextWindow{
		Sources: []qacontext.SourceReference{
			{CitationKey: "[Ref 1]", DocumentID: "doc-1", Content: "source one"},
			{CitationKey: "[Ref 2]", DocumentID: "doc-2", Content: "source two"},
		},
	}

	t.Run("one entailed pair among neutral pairs keeps the claim verified", func(t *testing.T) {
		checker := &fakeChecker{sourceResults: map[string]qaverification.EntailmentScore{
			"source one": {Score: 0.95, Relation: "entailed"},
			"source two": {Score: 0.3, Relation: "neutral"},
		}}
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "Claim [Ref 1] [Ref 2].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusVerified {
			t.Errorf("status = %q, want verified (one entailed source suffices)", ans.Status)
		}
	})

	t.Run("neutral-only claim is unverified", func(t *testing.T) {
		checker := &fakeChecker{sourceResults: map[string]qaverification.EntailmentScore{
			"source one": {Score: 0.2, Relation: "neutral"},
		}}
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "Claim [Ref 1].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("status = %q, want unverified (no entailed source)", ans.Status)
		}
	})

	t.Run("entailed plus contradicted pairs downgrade the claim", func(t *testing.T) {
		checker := &fakeChecker{sourceResults: map[string]qaverification.EntailmentScore{
			"source one": {Score: 0.9, Relation: "entailed"},
			"source two": {Score: 0.9, Relation: "contradicted"},
		}}
		pipeline := qaverification.NewDefaultVerificationPipeline()
		pipeline.SetEntailmentChecker(checker)

		ans, err := pipeline.Verify(ctx, "Claim [Ref 1] [Ref 2].", win)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if ans.Status != qaverification.StatusUnverified {
			t.Errorf("status = %q, want unverified (contradicted source is strict)", ans.Status)
		}
	})
}
