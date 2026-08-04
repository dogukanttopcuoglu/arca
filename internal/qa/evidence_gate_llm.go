package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmprovider "arca/internal/llm/provider"
	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
)

// evidenceGateSystemInstruction constrains the gate to emit a single
// structured decision; the gate and answer generation have different
// contracts and must not share a prompt (ADR-0034).
const evidenceGateSystemInstruction = `You are the ARC evidence gate. Decide whether the provided sources answer the user question.
Respond with ONLY a single JSON object: {"decision": "supported"} if the sources contain enough information to answer, or {"decision": "unsupported"} if they do not.
Never include any other text or explanation.`

// LLMEvidenceGate is the production EvidenceGate adapter speaking through the
// existing provider-neutral LLMProvider seam. It contains no provider-specific
// logic and never introduces a second provider abstraction.
type LLMEvidenceGate struct {
	llm llmprovider.LLMProvider
}

// NewLLMEvidenceGate constructs an EvidenceGate adapter over an LLMProvider.
func NewLLMEvidenceGate(llm llmprovider.LLMProvider) *LLMEvidenceGate {
	return &LLMEvidenceGate{llm: llm}
}

// Evaluate asks the provider whether the context answers the query and maps
// the response onto the typed contract. Only the explicit decision values
// "supported" and "unsupported" are accepted; malformed, missing, or
// ambiguous output is an operational EvidenceGateError, never a semantic
// conclusion (ADR-0034).
func (g *LLMEvidenceGate) Evaluate(ctx context.Context, query string, win *qacontext.ContextWindow) (EvidenceDecision, error) {
	if g.llm == nil {
		return EvidenceGateFailed, fmt.Errorf("evidence gate LLM provider is nil")
	}
	if strings.TrimSpace(query) == "" {
		return EvidenceGateFailed, fmt.Errorf("evidence gate query is empty")
	}
	if win == nil || strings.TrimSpace(win.Content) == "" {
		return EvidenceGateFailed, fmt.Errorf("evidence gate context is empty")
	}

	prompt := qaprompt.PromptMessage{
		System: evidenceGateSystemInstruction,
		Messages: []qaprompt.Message{
			{Role: "user", Content: fmt.Sprintf("SOURCES:\n%s\n\nQUESTION:\n%s", win.Content, query)},
		},
		Options: qaprompt.GenerationOptions{Temperature: 0, MaxTokens: 32},
	}

	resp, err := g.llm.Generate(ctx, prompt)
	if err != nil {
		return EvidenceGateFailed, fmt.Errorf("evidence gate generation failed: %w", err)
	}

	decision, err := parseEvidenceDecision(resp.Content)
	if err != nil {
		return EvidenceGateFailed, err
	}
	return decision, nil
}

// parseEvidenceDecision strictly parses the structured gate output. Any
// output outside the exact schema and explicit decision vocabulary is an
// operational error.
func parseEvidenceDecision(content string) (EvidenceDecision, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return EvidenceGateFailed, fmt.Errorf("evidence gate returned empty output")
	}

	var payload struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return EvidenceGateFailed, fmt.Errorf("evidence gate returned malformed JSON: %w", err)
	}
	if strings.TrimSpace(payload.Decision) == "" {
		return EvidenceGateFailed, fmt.Errorf("evidence gate output missing decision field")
	}

	switch strings.ToLower(strings.TrimSpace(payload.Decision)) {
	case string(EvidenceSupported):
		return EvidenceSupported, nil
	case string(EvidenceUnsupported):
		return EvidenceUnsupported, nil
	default:
		return EvidenceGateFailed, fmt.Errorf("evidence gate returned unrecognized decision %q", payload.Decision)
	}
}
