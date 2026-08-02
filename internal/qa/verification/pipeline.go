package verification

import (
	"context"
	"fmt"

	qacitation "arca/internal/qa/citation"
	qacontext "arca/internal/qa/context"
)

// VerifiedAnswer models a verified RAG answer payload with structural metrics.
type VerifiedAnswer struct {
	Text         string                       `json:"text"`
	Citations    []qacitation.AnswerCitation  `json:"citations"`
	IsVerified   bool                         `json:"is_verified"`
	Verification qacitation.VerificationReport `json:"verification"`
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

// Verify runs Phase 1 structural citation checks and produces a VerifiedAnswer.
func (p *DefaultVerificationPipeline) Verify(ctx context.Context, answerText string, win *qacontext.ContextWindow) (*VerifiedAnswer, error) {
	if answerText == "" {
		return nil, fmt.Errorf("answer text cannot be empty")
	}

	citations, report, err := p.extractor.Extract(answerText, win)
	if err != nil {
		return nil, err
	}

	isVerified := report.InvalidReferences == 0 && report.VerifiedClaims > 0

	return &VerifiedAnswer{
		Text:         answerText,
		Citations:    citations,
		IsVerified:   isVerified,
		Verification: report,
	}, nil
}
