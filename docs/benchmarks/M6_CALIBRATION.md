# M6 Evidence-Budget Calibration — Comparison TopK

**Date:** 2026-08-05
**Protocol:** ADR-0037 acceptance rule — comparison `unsupported` rate must drop while dense recall@5 and abstention recall stay within the 5% regression tolerance (M3 gates).
**Corpus:** Gold Set v2 (5 documents, 3017 chunks, fingerprint `8b21a664…`), dense, TopK 5 base, min-score 0.6, `arc eval --m5-gate`.
**Gate model:** OpenRouter `google/gemma-4-26b-a4b-it:free`.

## Results

| config | comparison unsupported | false abstentions | abstention precision | abstention recall | dense recall@5 | MRR | gate errors | generation skipped |
|---|---|---|---|---|---|---|---|---|
| TopK 5 (baseline `m5_gate_v1.json`) | 4/4 | 5 | 0.615 | 1.000 | 1.000 | 0.897 | 2 | 13 |
| **TopK 8 (`m6_gate_topk8.json`)** | **2/4** | **3** | **0.727** | **1.000** | **1.000** | 0.897 | **1** | **11** |
| TopK 10 (`m6_gate_topk10.json`) | rejected | — | — | — | — | — | 20 | — |

## Per-query comparison decisions

| query | TopK 5 | TopK 8 |
|---|---|---|
| m5-cmp-01 (bounded contexts vs subsystems) | unsupported | unsupported |
| m5-cmp-02 (event messaging vs feedback loops) | unsupported | unsupported |
| m5-cmp-03 (CQRS vs event-carried state transfer) | unsupported | **supported** |
| m5-cmp-04 (inspiration vs events) | unsupported | **supported** |

## Verdict

- **TopK 8 frozen as the production default** (`RETRIEVAL_COMPARISON_TOP_K=8`). Comparison false abstentions halve (4/4 → 2/4), abstention precision 0.615 → 0.727, abstention recall and dense recall@5 flat — acceptance rule passed.
- **TopK 10 rejected**: 20/29 gate calls failed operationally (10 upstream 429 rate limits on the free-tier shared pool, 9 malformed outputs, 1 empty response). The run is invalid as calibration evidence, not a gate-quality result.
- Remaining comparison false abstentions (m5-cmp-01/02) are deeper evidence-scarcity cases; no further budget increase is justified by this data — the next lever would be gate-prompt or context-assembly work, both outside M6 scope (ADR-0037).
- Free-tier model instability (429 + malformed outputs) remains the dominant gate-quality risk; deterministic fake-gate tests stay the regression anchor.
