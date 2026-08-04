---
title: 00 — M5 contracts
feature: m5-evidence-gate
status: ready-for-agent
created: 2026-08-04
blocked-by: ADR-0030, ADR-0031, ADR-0032, ADR-0033, ADR-0034
---

# 00 — M5 contracts

## Goal

Define the smallest public seams required by the confirmed M5 architecture.

## Scope

- Add the concrete comparison decision with only `decompose bool`.
- Add the `qa`-owned `EvidenceGate` seam.
- Define typed `supported`, `unsupported`, and `gate_error` outcomes.
- Define the typed terminal gate error used after one retry.
- Do not add `RetrievalPlan`, `IntentHint`, or a policy-aware retrieval seam.

## Files/modules affected

- `internal/qa/`
- `internal/qa/context/` only for the existing `ContextWindow` dependency if required

## Dependencies

- ADR-0030 through ADR-0034

## Acceptance criteria

- Contracts compile without changing retrieval interfaces.
- `EvidenceGate` accepts the original query and final `ContextWindow`.
- Outcomes distinguish semantic unsupported from operational failure.
- No production behavior changes yet.
