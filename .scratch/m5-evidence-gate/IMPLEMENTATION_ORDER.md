# M5 Implementation Order

The sequence preserves the M4 boundary and keeps each change independently testable.

1. **ADRs and tickets** — record the accepted M5 boundaries and dependencies.
2. **Contracts** — add the concrete comparison decision and the `qa`-owned `EvidenceGate` contract with typed outcomes.
3. **Tests** — add deterministic tests for supported, unsupported, malformed output, retry, fail-closed behavior, and generation avoidance.
4. **AnswerEngine integration** — consume `SubQueries`, preserve the existing merge path, evaluate the final `ContextWindow`, and skip or fail closed before generation.
5. **LLM adapter** — implement the provider-neutral structured adapter through the existing `LLMProvider` seam; keep real-provider results separate from deterministic tests.
6. **Runtime wiring** — provide a real gate from every production synchronous `AnswerEngine` composition root and align the production min-score default to `0.6`.
7. **Benchmark reporting** — add immutable M5 reports using Gold Set v1.1 and its fingerprint; record decisions, gate outcomes, retries, latency, cost, and avoided generation calls.

M5 must not introduce `RetrievalPlan`, `IntentHint`, runtime `DenseBiased` routing, fusion recalibration, a new Gold Set, or retrieval implementation changes.
