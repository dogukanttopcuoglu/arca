---
title: 02 — AnswerEngine orchestration integration
feature: m5-evidence-gate
status: ready-for-agent
created: 2026-08-04
blocked-by: 00 — M5 contracts; 01 — EvidenceGate behavior tests
---

# 02 — AnswerEngine orchestration integration

## Goal

Integrate comparison decomposition and pre-generation semantic abstention into the synchronous `AnswerEngine` path.

## Scope

- Consume non-empty `AnalyzedQuery.SubQueries` as the comparison signal.
- Keep `Balanced` retrieval and the existing `MergeRankedLists` behavior.
- Build the final `ContextWindow` before calling the gate.
- Preserve empty-retrieval behavior.
- Inject `EvidenceGate` explicitly; nil remains available only for tests/legacy composition.
- Do not modify streaming, MCP, agent, or retrieval implementations.

## Files/modules affected

- `internal/qa/engine.go`
- `internal/qa/` comparison orchestration helper
- `internal/qa/*_test.go`

## Dependencies

- 00 — M5 contracts
- 01 — EvidenceGate behavior tests

## Acceptance criteria

- Comparison requests use the existing sub-query retrieval and merge path.
- Unsupported context returns `StatusNoEvidence` without generation.
- Gate exhaustion returns a typed error and no `Answer`.
- Non-comparison behavior remains Balanced and non-decomposed.
