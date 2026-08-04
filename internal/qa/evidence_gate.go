package qa

import (
	"context"
	"fmt"

	qacontext "arca/internal/qa/context"
)

// EvidenceDecision is the typed outcome of pre-generation semantic evidence
// evaluation (ADR-0034). It distinguishes a semantic conclusion from an
// operational gate failure; a bool cannot carry that distinction.
type EvidenceDecision string

const (
	// EvidenceSupported marks context that sufficiently answers the query.
	EvidenceSupported EvidenceDecision = "supported"
	// EvidenceUnsupported marks context that does not answer the query;
	// AnswerEngine returns StatusNoEvidence and skips generation.
	EvidenceUnsupported EvidenceDecision = "unsupported"
	// EvidenceGateFailed marks an operational failure of the evidence
	// evaluation itself, not a statement about the context. AnswerEngine
	// retries once, then fails closed with an EvidenceGateError.
	EvidenceGateFailed EvidenceDecision = "gate_error"
)

// EvidenceGate is the pre-generation seam deciding whether the final
// assembled ContextWindow answers the original user query. It runs after
// context construction and before the answer-generation call, and is
// deliberately separate from post-generation EntailmentChecker verification
// (ADR-0033).
type EvidenceGate interface {
	// Evaluate returns a typed decision for the original query against the
	// exact context that would be sent to generation. A non-nil error is an
	// operational failure and must not be interpreted as a semantic outcome.
	Evaluate(ctx context.Context, query string, win *qacontext.ContextWindow) (EvidenceDecision, error)
}

// EvidenceGateError is the typed terminal failure returned by AnswerEngine
// when the evidence gate exhausts its bounded retries (ADR-0034). It always
// wraps the last operational cause so callers can distinguish an unavailable
// gate from an unsupported-context abstention.
type EvidenceGateError struct {
	// Attempts is the total number of Evaluate calls attempted (1 + retries).
	Attempts int
	// Cause is the last underlying operational failure, or nil when the gate
	// returned EvidenceGateError without an error.
	Cause error
}

// Error implements the error interface.
func (e *EvidenceGateError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("evidence gate failed after %d attempts: %v", e.Attempts, e.Cause)
	}
	return fmt.Sprintf("evidence gate failed after %d attempts", e.Attempts)
}

// Unwrap exposes the underlying operational cause.
func (e *EvidenceGateError) Unwrap() error {
	return e.Cause
}

// MaxGateAttempts is the bounded retry budget: one initial attempt plus one
// retry (ADR-0034). No configuration surface is exposed before operational
// evidence justifies tuning it. The benchmark harness references this
// constant so its mirror policy cannot drift from the engine.
const MaxGateAttempts = 2
