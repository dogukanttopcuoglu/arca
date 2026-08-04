---
title: 03 — LLM EvidenceGate adapter
feature: m5-evidence-gate
status: ready-for-agent
created: 2026-08-04
blocked-by: 00 — M5 contracts; 02 — AnswerEngine orchestration integration
---

# 03 — LLM EvidenceGate adapter

## Goal

Provide one provider-neutral production adapter for semantic evidence evaluation.

## Scope

- Use the existing `LLMProvider` seam.
- Add a dedicated evidence prompt and strict structured decision parser.
- Accept only explicit `supported` or `unsupported` values.
- Convert malformed, missing, or ambiguous output to `gate_error`.
- Record provider/model metadata where the existing reporting seam permits it.

## Files/modules affected

- `internal/qa/` EvidenceGate adapter and prompt
- `internal/llm/provider/` only if an existing provider-neutral capability is required
- Adapter tests

## Dependencies

- 00 — M5 contracts
- 02 — AnswerEngine orchestration integration

## Acceptance criteria

- The adapter has no provider-specific dependency beyond `LLMProvider`.
- Valid structured responses map deterministically to the two semantic outcomes.
- Invalid responses become operational gate errors.
- Real-provider evaluation remains separate from deterministic unit tests.
