# M8 reranking seam & execution contract: ordering-only Reranker seam behind a wrapping retriever

Status: accepted

Reranking enters the system as two distinct layers: a `Reranker` seam that abstracts only model behavior, and a `RerankedRetriever` execution component that wraps any `seam.Retriever`. The contract between them is an ordering contract, never a score contract.

## Decisions

- **`Reranker` seam.** A deep-module seam abstracting model behavior only (e.g. `Rerank(ctx, query, candidates) ([]ScoredCandidate, error)`). Model implementations (e.g. a cross-encoder) are adapters of this seam; the seam knows nothing about retrieval, candidate budgets, or truncation.
- **`RerankedRetriever` execution component.** A wrapper implementing `seam.Retriever`. Responsibilities: requesting the candidate budget N from the inner retriever, calling the `Reranker`, truncating to the caller's TopK, error handling, and deterministic final ordering. The outside world still sees a standard `Retriever` that returns TopK results — the candidate budget is the wrapper's internal behavior only and never changes the retrieval contract.
- **Ordering contract (normative).** The `Reranker` produces an ordering, not scores:
  - Absolute scores carry no meaning; the wrapper never interprets or normalizes them.
  - Adapters do not need to share a score scale (cross-encoder logits are not comparable across model families).
  - The only guarantee is deterministic ordering: same query + same candidate list -> same output ordering, with tie-break by ChunkID ASC.
- **No filtering.** The reranker never filters: interpreting absolute scores for thresholding is forbidden by the ordering contract. Candidate generation and `RETRIEVAL_MIN_SCORE` filtering stay entirely in the inner retriever. Consequence: abstention queries are preserved — an empty candidate list stays empty through rerank, and `expected_no_evidence` behavior is unchanged.
- **No write-back to fusion.** The reranker does not write scores back into fusion streams; it only changes the final ordering. `FusionPolicy` and fusion score handling (ADR-0041) are untouched.
- **Wrapper vs wiring (normative distinction).** `RerankedRetriever` is a domain execution component that can wrap any `Retriever` (dense, graph fusion, future pipelines). Which slot it is wired into in production is a separate decision made from benchmark evidence (ADR-0046) — the wrapper is generic, the wiring is path-specific. Future pipelines can be benchmarked by changing only the composition, without new abstractions.
- **Configuration.** `RerankedRetriever` config: model identifier + candidate budget N. Both are wrapper-internal; callers and AnswerEngine keep TopK semantics unchanged.

## Rationale

- Continues the proven seam -> adapter -> wrapper architecture from M5 (EvidenceGate) and M7 (GraphStore): adapters swap under the seam, and the wrapper, orchestration, benchmark harness, and M7 contracts stay fixed.
- Approach comparisons stay clean: after the probe only the adapter changes, which keeps results comparable and isolates the reranker's contribution.
- Determinism (Q4 contract) makes benchmarks reproducible: same input, same ordering.

## Consequences

- `internal/retrieval/rerank/` (or equivalent) contains the seam, the wrapper, and adapters; the wrapper is composable with any inner retriever.
- ADR-0045 defines how the two candidates are measured; ADR-0046 defines where the wrapper is wired after acceptance.
