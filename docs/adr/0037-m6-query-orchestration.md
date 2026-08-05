# M6 query orchestration: minimal IntentHint, pure decision orchestrator, benchmark-gated evidence budget

Status: accepted

M6 productizes only benchmark-proven retrieval decisions. It defines a minimal `IntentHint` produced from the existing deterministic analyzer signal, a pure decision-function `Retrieval Orchestrator` in `qa`, and a single benchmark-gated evidence-budget mechanism (TopK override) aimed at the measured comparison false-abstention signal from M5 validation (4 of 4 comparison queries gate-`unsupported` at TopK 5). Runtime fusion-policy selection is explicitly not introduced: no benchmark on the current production path (dense, Gold Set v2) shows a per-intent gain from switching policies.

## Decisions

- **Scope boundary:** M6 owns `IntentHint` production, `Retrieval Orchestrator` selection, query decomposition behavior, and evidence-budget management. Explicitly out of M6: LLM intent classifiers, rerankers, KG fusion, agentic retrieval, and any change to the M5 `EvidenceGate`. The gate is not the proven problem — the retrieval context it receives is; M6 improves what the gate receives, not how the gate works.
- **Minimal IntentHint:** the hint carries only `Intent`, `Decompose`, and `Source` (initially `"rule_based"`), derived from the existing `QueryAnalyzer` signal (`SubQueries` non-empty → comparison). No confidence score: probabilistic semantics are not invented before a benchmarked classifier justifies them. Retrieval-derived signals (scores, DF) do not enter the hint.
- **Orchestrator shape:** a pure decision function in `qa`: `IntentHint + RuntimeConfig → RetrievalDecision`. `AnswerEngine` executes the decision; retrievers remain execution components. The orchestrator has no side effects — policy-bound retriever selection stays in composition.
- **No runtime policy selection:** `RetrievalDecision` has no `Policy` field. Fusion policy remains config-frozen (`Balanced`). M4's `DenseBiased` evidence was measured in hybrid mode on the rick-rubin corpus; on the current production path (dense + decomposition, Gold Set v2) dense achieves recall@5 1.000 while hybrid + decomposition measures 0.833 — no per-intent gap exists for a second policy to close. Runtime policy selection reopens only when a production benchmark demonstrates a per-intent improvement beyond the 5% regression tolerance.
- **Evidence budget = TopK override only:** `RetrievalDecision` may carry an optional `TopKOverride`; the default is the caller's TopK. Only benchmark-proven intents (comparison initially) may override it. The concrete value (e.g. 8 or 10) is not hardcoded now — it must come from benchmarking false-abstention reduction on Gold Set v2 before the number freezes. Token-budget changes and gate redesign are out of scope.
- **Comparison handling:** the existing decomposition pipeline is unchanged (`SubQueries` → per-sub-query `Retrieve` → `MergeRankedLists`). `TopKOverride` is the only comparison-specific behavior and applies consistently: each sub-query retrieves `TopKOverride`, merge trims to `TopKOverride` — preserving the existing `MergeRankedLists(topK)` contract and avoiding multiple budget knobs.

## Rationale

- Every decision is gated by measured evidence: the M5 validation signal (4/4 comparison false abstentions) motivates the single new mechanism (TopK override), and the absence of any measured per-intent policy gap motivates keeping policy config-frozen. Adding an unused policy branch would repeat the M5 lesson against speculative machinery.
- Keeping the orchestrator a pure function preserves the M5 seam discipline: hint production (`QueryAnalyzer`), decision (`qa`), and execution (`AnswerEngine` + retrievers) remain separately testable, and the decision surface stays minimal until benchmark results justify expansion.

## Consequences

- `RetrievalDecision` initially exposes only benchmark-validated decision fields (`Decompose bool` and optional `TopKOverride`); additional fields (e.g. a `Policy`) require benchmark evidence demonstrating a second runtime policy path before they are added.
- The `IntentHint` and `Retrieval Orchestrator` glossary entries in `CONTEXT.md` are updated to reflect the minimal, benchmark-gated semantics.
- M6 calibration benchmark: run `arc eval --m5-gate` on Gold Set v2 with comparison TopK overrides and verify comparison `unsupported` rate drops while dense recall@5 and abstention recall stay within the 5% regression tolerance (M3 gates).
- M6 results are measured against the frozen M5 artifacts: `M5_VALIDATION.md`, `m5_gate_v1.json`, Gold Set v2, and the M4 fusion sweep reports.
