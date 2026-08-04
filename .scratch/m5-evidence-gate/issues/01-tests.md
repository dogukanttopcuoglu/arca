---
title: 01 — EvidenceGate behavior tests
feature: m5-evidence-gate
status: ready-for-agent
created: 2026-08-04
blocked-by: 00 — M5 contracts
---

# 01 — EvidenceGate behavior tests

## Goal

Lock the M5 safety behavior at the agreed seams before integrating production behavior.

## Scope

- Add deterministic fake-gate tests for supported and unsupported context.
- Verify unsupported context skips answer generation and returns `StatusNoEvidence`.
- Verify gate errors retry exactly once, then fail closed with a typed error.
- Verify malformed decisions are operational gate errors.
- Verify empty retrieval preserves the existing immediate no-evidence path.

## Files/modules affected

- `internal/qa/*_test.go`
- Existing LLM test doubles only

## Dependencies

- 00 — M5 contracts

## Acceptance criteria

- Tests observe behavior through `AnswerEngine` and `EvidenceGate` seams.
- Generation-call avoidance is asserted.
- Retry count is exactly two attempts maximum.
- Tests do not require a live provider.
