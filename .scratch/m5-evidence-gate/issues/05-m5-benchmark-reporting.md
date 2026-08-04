---
title: 05 — M5 orchestration and abstention reports
feature: m5-evidence-gate
status: ready-for-agent
created: 2026-08-04
blocked-by: 02 — AnswerEngine orchestration integration; 03 — LLM EvidenceGate adapter; 04 — Production EvidenceGate wiring
---

# 05 — M5 orchestration and abstention reports

## Goal

Measure M5 behavior against immutable M4 artifacts without retuning retrieval.

## Scope

- Use Gold Set v1.1 unchanged.
- Verify corpus fingerprint `51b1909e...` before evaluation.
- Record decomposition use, selected Balanced path, gate decision, retries, latency, cost, and generation avoidance.

Known limitations (documented in the report, not silently omitted):
- "Selected Balanced path" is implied by the manifest (fusion policy) plus the per-query `decomposed` flag; Balanced is the only runtime path in M5.
- Gate token cost is not recorded: the `EvidenceGate` contract does not expose usage, and the harness must not assume an LLM-backed implementation. Cost accounting is deferred until the seam gains an explicit usage surface.
- Empty retrieval abstains without a gate observation (mirrors the engine's immediate no_evidence path); M5 abstention metrics still count it as generation-skipped.
- Produce separate deterministic-fake and real-provider reports.
- Measure abstention precision/recall, false abstentions, missed abstentions, and retrieval/context coverage.

## Files/modules affected

- `internal/eval/`
- `docs/benchmarks/` new M5 reports
- Evaluation tests

## Dependencies

- 02 — AnswerEngine orchestration integration
- 03 — LLM EvidenceGate adapter
- 04 — Production EvidenceGate wiring

## Acceptance criteria

- Gold Set and all M4 reports remain unchanged.
- Fingerprint mismatch aborts evaluation before queries run.
- Deterministic and real-provider results are clearly separated.
- Reports identify the query-level-label limitation of `expected_no_evidence`.
- No fusion weights, retrieval modes, or Gold Set definitions change.
