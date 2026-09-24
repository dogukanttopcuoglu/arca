package verification

import (
	"context"
	"fmt"
	"os"

	qacitation "arca/internal/qa/citation"
	qacontext "arca/internal/qa/context"
)

// VerificationStatus is the explicit evidence state of an Answer, carried on
// the answer itself rather than inferred from citation counts.
type VerificationStatus string

const (
	// StatusVerified marks an answer with at least one valid citation and no invalid references.
	StatusVerified VerificationStatus = "verified"
	// StatusUnverified marks a generated answer with invalid references or no citations at all.
	StatusUnverified VerificationStatus = "unverified"
	// StatusNoEvidence marks answers produced without any retrieved sources (generation skipped).
	StatusNoEvidence VerificationStatus = "no_evidence"
)

// VerifiedAnswer models a verified RAG answer payload with structural metrics.
type VerifiedAnswer struct {
	Text         string                        `json:"text"`
	Citations    []qacitation.AnswerCitation   `json:"citations"`
	Status       VerificationStatus            `json:"status"`
	Verification qacitation.VerificationReport `json:"verification"`
	// Degraded marks a Phase 2 external-service failure: the Phase 1 status
	// stands and semantics never downgrade an answer.
	Degraded bool `json:"degraded"`
	// Semantic carries one verdict per (claim sentence, source) pair when
	// Phase 2 ran to completion.
	Semantic []SemanticVerdict `json:"semantic"`
}

// SemanticVerdict records one Phase 2 entailment check for a single
// (claim sentence, source) pair.
type SemanticVerdict struct {
	Claim    string  `json:"claim"`
	Score    float64 `json:"score"`
	Relation string  `json:"relation"`
}

// EntailmentScore models semantic NLI entailment results for Phase 2 verification.
type EntailmentScore struct {
	Score    float64 `json:"score"`
	Relation string  `json:"relation"` // "entailed", "contradicted", "neutral"
}

// EntailmentChecker defines the Phase 2 seam for NLI semantic entailment checks.
type EntailmentChecker interface {
	CheckEntailment(ctx context.Context, claim, sourceText string) (EntailmentScore, error)
}

// VerificationPipeline defines the seam for executing Phase 1 (Structural) and Phase 2 (Entailment) verification.
type VerificationPipeline interface {
	Verify(ctx context.Context, answerText string, win *qacontext.ContextWindow) (*VerifiedAnswer, error)
}

// DefaultVerificationPipeline implements VerificationPipeline executing Phase 1 structural reference extraction.
type DefaultVerificationPipeline struct {
	extractor qacitation.CitationExtractor
	checker   EntailmentChecker
}

// NewDefaultVerificationPipeline constructs a DefaultVerificationPipeline instance.
func NewDefaultVerificationPipeline() *DefaultVerificationPipeline {
	return &DefaultVerificationPipeline{
		extractor: qacitation.NewDefaultCitationExtractor(),
	}
}

// SetEntailmentChecker attaches an optional Phase 2 EntailmentChecker.
func (p *DefaultVerificationPipeline) SetEntailmentChecker(checker EntailmentChecker) {
	p.checker = checker
}

// Verify runs Phase 1 structural citation checks and, when an entailment
// checker is attached, Phase 2 semantic checks per (claim sentence, source)
// pair. A Phase 2 failure fails open: the Phase 1 status stands, Degraded is
// set, and the error is never propagated as a pipeline failure.
func (p *DefaultVerificationPipeline) Verify(ctx context.Context, answerText string, win *qacontext.ContextWindow) (*VerifiedAnswer, error) {
	if answerText == "" {
		return nil, fmt.Errorf("answer text cannot be empty")
	}

	citations, report, err := p.extractor.Extract(answerText, win)
	if err != nil {
		return nil, err
	}

	status := StatusUnverified
	if report.InvalidReferences == 0 && report.VerifiedClaims > 0 {
		status = StatusVerified
	}

	answer := &VerifiedAnswer{
		Text:         answerText,
		Citations:    citations,
		Status:       status,
		Verification: report,
	}

	p.runPhase2(ctx, answer, win)
	return answer, nil
}

// runPhase2 checks every (claim sentence, source) pair against the attached
// checker. Refs already counted invalid in Phase 1 have no source in win and
// are skipped. On the first checker error Phase 2 stops, the Phase 1 status
// is kept, and Degraded is set; no_evidence status is never touched.
func (p *DefaultVerificationPipeline) runPhase2(ctx context.Context, answer *VerifiedAnswer, win *qacontext.ContextWindow) {
	if p.checker == nil || win == nil {
		return
	}

	sourceByKey := make(map[string]qacontext.SourceReference, len(win.Sources))
	for _, src := range win.Sources {
		sourceByKey[src.CitationKey] = src
	}

	claims := qacitation.ExtractClaims(answer.Text)
	verdicts := make([]SemanticVerdict, 0, len(claims))
	for _, claim := range claims {
		for _, ref := range claim.Refs {
			src, exists := sourceByKey[fmt.Sprintf("[Ref %d]", ref)]
			if !exists {
				continue
			}
			score, err := p.checker.CheckEntailment(ctx, claim.Sentence, src.Content)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: semantic entailment check failed, keeping structural status: %v\n", err)
				answer.Degraded = true
				return
			}
			verdicts = append(verdicts, SemanticVerdict{Claim: claim.Sentence, Score: score.Score, Relation: score.Relation})
		}
	}
	answer.Semantic = verdicts

	if answer.Status == StatusNoEvidence {
		return
	}
	for _, verdict := range verdicts {
		if verdict.Relation != "entailed" {
			answer.Status = StatusUnverified
			return
		}
	}
}
