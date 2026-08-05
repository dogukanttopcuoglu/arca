# M6 query orchestration: minimal IntentHint, pure decision orchestrator, benchmark-gated evidence budget

Status: accepted

M6 productizes only benchmark-proven retrieval decisions. It defines a minimal `IntentHint` produced from the existing deterministic analyzer signal, a pure decision-function `Retrieval Orchestrator` in `qa`, and a single benchmark-gated evidence-budget mechanism (TopK override) aimed at the measured comparison false-abstention signal from M5 validation (4 of 4 comparison queries gate-`unsupported` at TopK 5). Runtime fusion-policy selection is explicitly not introduced: no benchmark on the current production path (dense, Gold Set v2) shows a per-intent gain from switching policies.

## Decisions

- **Scope boundary:** M6 owns `IntentHint` production, `Retrieval Orchestrator` selection, query decomposition behavior, and evidence-budget management. Explicitly out of M6: LLM intent classifiers, rerankers, KG fusion, agentic retrieval, and any change to the M5 `EvidenceGate`. The gate is not the proven problem — the retrieval context it receives is; M6 improves what the gate receives, not how the gate works.
- **Minimal IntentHint:** the hint carries only `Intent`, `Decompose`, and `Source` (initially `"rule_based"`), derived from the existing `QueryAnalyzer` signal (`SubQueries` non-empty → comparison). No confidence score: probabilistic semantics are not invented before a benchmarked classifier justifies them. Retrieval-derived signals (scores, DF) do not enter the hint.
- **Orchestrator shape:** a pure decision function in `qa`: `IntentHint + RuntimeConfig → RetrievalDecision`. `AnswerEngine` executes the decision; retrievers remain execution components. The orchestrator has no side effects — policy-bound retriever selection stays in composition.
- **No runtime policy selection:** `RetrievalDecision` has no `Policy` field. Fusion policy remains config-frozen (`Balanced`). M4's `DenseBiased` evidence was measured in hybrid mode on the rick-rubin corpus; on the current production path (dense + decomposition, Gold Set v2) dense achieves recall@5 1.000 while hybrid + decomposition measures 0.833 — no per-intent gap exists for a second policy to close. Runtime policy selection reopens only when a production benchmark demonstrates a per-intent improvement beyond the 5% regression tolerance.
- **Evidence budget = TopK override only:** `RetrievalDecision` may carry an optional `TopKOverride`; the default is the caller's TopK. Only benchmark-proven intents (comparison initially) may override it. The concrete value is not hardcoded until benchmarked: **calibrated to 8 on Gold Set v2** (see Calibration below) and frozen as the production default (`RETRIEVAL_COMPARISON_TOP_K=8`). Token-budget changes and gate redesign are out of scope.
- **Comparison handling:** the existing decomposition pipeline is unchanged (`SubQueries` → per-sub-query `Retrieve` → `MergeRankedLists`). `TopKOverride` is the only comparison-specific behavior and applies consistently: each sub-query retrieves `TopKOverride`, merge trims to `TopKOverride` — preserving the existing `MergeRankedLists(topK)` contract and avoiding multiple budget knobs.

## Rationale

- Every decision is gated by measured evidence: the M5 validation signal (4/4 comparison false abstentions) motivates the single new mechanism (TopK override), and the absence of any measured per-intent policy gap motivates keeping policy config-frozen. Adding an unused policy branch would repeat the M5 lesson against speculative machinery.
- Keeping the orchestrator a pure function preserves the M5 seam discipline: hint production (`QueryAnalyzer`), decision (`qa`), and execution (`AnswerEngine` + retrievers) remain separately testable, and the decision surface stays minimal until benchmark results justify expansion.

## Consequences

- `RetrievalDecision` initially exposes only benchmark-validated decision fields (`Decompose bool` and optional `TopKOverride`); additional fields (e.g. a `Policy`) require benchmark evidence demonstrating a second runtime policy path before they are added.
- The `IntentHint` and `Retrieval Orchestrator` glossary entries in `CONTEXT.md` are updated to reflect the minimal, benchmark-gated semantics.
- M6 calibration benchmark: run `arc eval --m5-gate` on Gold Set v2 with comparison TopK overrides and verify comparison `unsupported` rate drops while dense recall@5 and abstention recall stay within the 5% regression tolerance (M3 gates).
- M6 results are measured against the frozen M5 artifacts: `M5_VALIDATION.md`, `m5_gate_v1.json`, Gold Set v2, and the M4 fusion sweep reports.

## Calibration (Gold Set v2, dense, min-score 0.6, `--m5-gate`)

| config | comparison unsupported | false abstentions | abstention precision | abstention recall | dense recall@5 | gate errors |
|---|---|---|---|---|---|---|
| TopK 5 (baseline `m5_gate_v1.json`) | 4/4 | 5 | 0.615 | 1.000 | 1.000 | 2 |
| **TopK 8 (frozen)** | **2/4** | **3** | **0.727** | **1.000** | **1.000** | **1** |
| TopK 10 | rejected | — | — | — | — | 20 |

TopK 8 passes the acceptance rule: comparison `unsupported` drops 4/4 → 2/4 while dense recall@5 and abstention recall stay flat. TopK 10 is rejected — 20 of 29 gate calls failed (10 upstream 429 rate limits on the free-tier pool, 9 malformed outputs, 1 empty), invalidating the run. Reports: `m6_gate_topk8.json`, `m6_gate_topk10.json` (unusable), `M6_CALIBRATION.md`.

**Model dependence (DeepSeek rerun):** with `deepseek-v4-flash` as the gate model the override shows no measurable gain (TopK 5/8/10 all at 0.727 precision, 3 false abstentions) — the benefit measured with gemma is model-dependent, and DeepSeek reaches the same quality at TopK 5. The frozen value stays 8 (no measured regression; evidence to remove it would need one). Per-model budgets (Gemma=8, DeepSeek=5) are not introduced; each would require its own calibration benchmark first. The gate `MaxTokens` was raised 32 → 256 for reasoning-capable providers (a provider-capability adjustment, not a gate-design change).
