# M7 orchestration gate: evidence-gated UseGraph, activation after benchmark acceptance

Status: accepted

The graph retrieval path is activated through the M6 orchestration layer only after benchmark acceptance — never before. Until ADR-0040's acceptance rule passes, the M6 contracts are untouched and production behavior is byte-identical to today.

## Decisions

- **Two-stage gate.** Without ADR-0040 acceptance, `RetrievalDecision` does not change. After acceptance: `IntentHint` gains the `entity` intent and `RetrievalDecision` gains `UseGraph bool` — the legitimate exercise of ADR-0037's rule ("additional fields require benchmark evidence"; the evidence is the accepted benchmark).
- **Runtime gate.** `UseGraph = (intent == entity) ∧ (graphWeight > 0)`. Entity queries route to the graph fusion path; every other query keeps the existing M6 path unchanged.
- **Activation vs rejection.** Acceptance: `HintIntentEntity` + `UseGraph` are added to code, and the production config `RETRIEVAL_GRAPH_WEIGHT` is introduced with the frozen ADR-0041 calibration value. Rejection (no sweep point accepts): none of these are added; the M6 orchestration contract stays as-is; the eval flags (`--graph-weight`, `--graph-only`) remain as an experiment surface (ADR-0041 rejection behavior). Distinction: eval flags are for measurement; the production config exists only for post-evidence activation — they never share a decision mechanism.
- **Composition.** `AnswerEngine` gains a second retriever slot via `WithGraphRetriever(r)`: `decision.UseGraph && graphRetriever != nil` executes retrieval through the graph fusion retriever, otherwise through the existing retriever — decomposition and `TopKOverride` behavior are unchanged, only the execution component switches. Composition root: when `RETRIEVAL_GRAPH_WEIGHT > 0` the runtime builds `GraphFusionRetriever` and injects it; at 0 nothing is injected (pre-acceptance and rejection behavior identical to today's binary). No orchestration information leaks into `RetrievalQuery`; intent analysis never runs twice — `RetrievalDecision` is the single decision source.

## Rationale

- "No evidence, no code" is preserved: a dead-code decision path is not introduced; rejection keeps the current behavior byte-identical.
- The engine decides, retrievers execute: the decision is made once in the orchestrator, and the execution component switch stays inside the engine's existing retrieval flow.

## Consequences

- This closes the M7 wayfinder map (tickets 01–08, ADR-0038…0042). Next: `/to-spec` collapses the decisions into a buildable plan, `/to-tickets` splits vertical slices, `/implement` builds them TDD-first — with the ADR-0040 benchmark and the ADR-0041 sweep as the acceptance gate.
