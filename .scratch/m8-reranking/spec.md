# M8 Reranking: Probe & Activation

Status: ready-for-agent

## Problem Statement

The production retrieval pipeline (dense + sparse + graph fusion, orchestration gate) returns TopK results directly. Ranking quality is the current bottleneck: on Gold Set v3 the fusion path reaches nDCG@5 = 0.879 and MRR = 0.938 (entity queries), and the research report (`docs/research/rag-reranking-architectures.md`) shows that second-stage reranking is the standard post-retrieval ordering correction used in production RAG systems (BAAI top-100->top-3, Azure top-50). But ARC has no evidence that a reranker improves its own pipeline — and the project discipline (M4-M7) is that measurement decides, assumption never does. An unmeasured reranker added to production would violate that discipline and could silently change retrieval behavior.

## Solution

Add an optional second-stage reranking layer behind a `RerankedRetriever` wrapper that implements the existing `seam.Retriever`: it internally requests a candidate budget N from the inner retriever (the M7 `GraphFusionRetriever`), reranks via a `Reranker` seam, and returns the caller's TopK. The approach is chosen by an offline probe measuring two architecture families (cross-encoder: BGE-reranker-v2-m3; late-interaction: ColBERTv2) at N = 20/50/100 against the unchanged production baseline. Acceptance is decided by frozen, baseline-relative thresholds (MPI: nDCG@5 delta >= +1pp; MAR: MRR delta >= -0.5pp, verified >= -1pp; abstention hard invariant). If no combination accepts, M8 closes with production byte-identical to today. On acceptance, the wrapper is wired into the graph slot via composition, controlled by `RETRIEVAL_RERANK_MODEL` (empty = off) and `RETRIEVAL_RERANK_CANDIDATE_N`, locked to the benchmark-accepted fingerprint.

## User Stories

1. As a retrieval engineer, I want the probe to run against the unchanged production baseline (GraphFusionRetriever, w=1.0, K=5) with rerank as the only variable, so that the measured delta represents only the reranker's contribution.
2. As a retrieval engineer, I want candidate lists recorded as benchmark artifacts (fingerprint, query id, ordered candidate ids, baseline metadata, candidate scores informational-only, reranker output ordering), so that different rerankers can be re-run over identical candidates and retrieval variance is fully eliminated.
3. As a retrieval engineer, I want to sweep N = 20/50/100 across both model candidates (6 combinations + baseline) and have per-combination metrics (nDCG@5, MRR, answer quality, p50/p95 latency, memory, cold/warm load) reported in one pass.
4. As a retrieval engineer, I want the kill gate evaluated automatically against the frozen thresholds (MPI +1pp nDCG@5; MAR -0.5pp MRR, -1pp verified; abstention identical), so that acceptance/rejection is deterministic and pre-announced.
5. As a retrieval engineer, I want "none accepted" to be a valid probe outcome that closes M8 with production unchanged, so that the probe can never be used to justify a predetermined solution.
6. As a retrieval engineer, I want a `Reranker` seam whose contract is an ordering contract — absolute scores carry no meaning, adapters need not share a score scale, the only guarantee is deterministic ordering with ChunkID ASC tie-break — so that cross-encoder and late-interaction adapters are interchangeable.
7. As a retrieval engineer, I want the `RerankedRetriever` to look like a standard `Retriever` from the outside (caller asks TopK=K, gets K), with the candidate budget N as wrapper-internal behavior, so that AnswerEngine, orchestration, and callers never change.
8. As a retrieval engineer, I want the reranker to never filter candidates (no score interpretation, no thresholding), so that abstention queries keep their `no_evidence` behavior as a hard invariant.
9. As a retrieval engineer, I want the reranker to never write scores back into fusion streams, so that `FusionPolicy` and fusion score handling stay untouched.
10. As a retrieval engineer, I want reranker failures to degrade gracefully to the inner retriever's result with a diagnostic, so that a reranker outage never breaks answering.
11. As a retrieval engineer, I want deterministic reranking (same query + same candidates -> same ordering), so that benchmark results are reproducible under the same fingerprint.
12. As a retrieval engineer, I want the probe to run entirely inside the existing evaluation harness (Gold Set v3, corpus fingerprint, ADR-0027 discipline), so that no new evaluation framework is introduced.
13. As an operations engineer, I want `RETRIEVAL_RERANK_MODEL=""` to mean the feature is off (no separate ENABLED flag), so that activation is a config value and rollback is a single config change.
14. As an operations engineer, I want the production configuration locked to the benchmark-accepted fingerprint (model, CandidateN), so that a parameter change requires a new benchmark and an old acceptance never carries over silently.
15. As an operations engineer, I want reranking wired only over the graph fusion path (the path the benchmark measured), leaving the dense path untouched, so that benchmark behavior and live behavior cannot diverge.
16. As an operations engineer, I want operational budgets (p95 latency, memory) frozen in the benchmark manifest before the probe starts, so that acceptance includes an operational ceiling, not just quality.
17. As a QA engineer, I want answer quality (EvidenceGate verified rate) measured on every combination, so that a reranker that improves ranking but degrades answers is rejected.
18. As a QA engineer, I want bootstrap CI reported for the delta (report-only, never a decision criterion), so that the report is informative without making the small gold set's sample size the decision maker.
19. As a domain engineer, I want the wrapper/wiring distinction recorded normatively (wrapper is generic and wraps any retriever; wiring over GraphFusion is a benchmark result), so that future pipelines are benchmarked by composition change only.
20. As a domain engineer, I want the deferred deployment decision (in-process inference vs sidecar service) recorded as out of scope, so that it is a separate, explicit follow-up decision after acceptance.

## Implementation Decisions

- **`Reranker` seam (ADR-0044).** A new deep-module seam abstracting model behavior only: `Rerank(ctx, query, candidates) ([]ScoredCandidate, error)`-shaped surface. Adapters: cross-encoder (BGE-reranker-v2-m3) and late-interaction (ColBERTv2), both probe-side. The seam owns the ordering contract: no absolute-score interpretation, adapters need not share a score scale, deterministic ordering with ChunkID ASC tie-break is the only guarantee.
- **`RerankedRetriever` execution component (ADR-0044).** Implements the existing `seam.Retriever`. Internally: request candidate budget N from the inner retriever, call the `Reranker`, truncate to the caller's TopK, deterministic final ordering, error handling. Reranker failure degrades to the inner retriever's result with a diagnostic (Graceful Degradation principle). Never filters (no thresholding — `RETRIEVAL_MIN_SCORE` stays in the inner retriever). Never writes scores back into fusion.
- **Composition wiring (ADR-0046).** `AnswerEngine` wiring gains the wrapper by composition: the graph slot receives `NewRerankedRetriever(graphFusionRetriever, rerankConfig)`. No engine concept changes; the M7 orchestration gate, `RetrievalDecision`, `FusionPolicy`, and the dense path are untouched. The wrapper is generic; its wiring over the graph fusion path is a benchmark result (normative wrapper/wiring distinction).
- **Configuration (ADR-0046).** `RETRIEVAL_RERANK_MODEL` (empty = feature off; no separate enabled flag) and `RETRIEVAL_RERANK_CANDIDATE_N` (wrapper-internal candidate budget; TopK semantics unchanged). Rollback is a single config change. Benchmark fingerprint and production config must match one-to-one (release criterion): different model or N invalidates the acceptance and requires a new benchmark.
- **Probe tooling (ADR-0045).** Extends the existing eval harness (ADR-0027): baseline run with candidate artifact recording; per-combination simulation runs (2 models x 3 N) over the recorded artifacts; MPI/MAR gate evaluation; benchmark manifest (fingerprint, gold set version, operational budgets frozen before the probe). Candidate artifact format is normative (ADR-0045): fingerprint, query id, ordered candidate ids, baseline retrieval metadata, candidate scores informational-only, reranker output ordering.
- **Acceptance thresholds (ADR-0045).** Baseline-relative, frozen before the benchmark: MPI (minimum practical improvement) nDCG@5 delta >= +1pp; MAR (maximum acceptable regression) MRR delta >= -0.5pp, verified-rate delta >= -1pp; abstention behavior identical (hard invariant); operational budgets (p95 latency, memory) not exceeded. Selection rule: highest quality among accepted combinations wins; within 5% tolerance, smallest N wins.
- **Closure (ADR-0043).** If no combination accepts, M8 closes: no production change, closure recorded in an ADR. The model pool never auto-expands; a third candidate requires a new decision. LLM rerankers remain out of scope.

## Testing Decisions

Good tests exercise external behavior through the seams, never implementation details: the `RerankedRetriever` is tested through the `seam.Retriever` surface with fake inner retriever and fake reranker; the `Reranker` seam is tested through its ordering contract; the gate is tested through the eval harness.

- **`RerankedRetriever` (via `seam.Retriever`):** requests N candidates from the inner retriever regardless of the caller's K; returns exactly K results; deterministic ordering including ChunkID ASC tie-break; reranker failure falls back to the inner result with diagnostics; empty candidate list stays empty (abstention preservation); caller TopK semantics unchanged.
- **`Reranker` seam:** same query + same candidates -> same ordering; two fake adapters producing different score scales yield the same ordering for the same preferences (scale independence); tie-break determinism.
- **Probe/eval harness:** candidate artifact generation is deterministic (same fingerprint, same artifact); MPI/MAR gate arithmetic is unit-tested; abstention queries reported identically with rerank on/off.
- **Prior art:** existing graph fusion retriever tests, eval runner tests, and M7 calibration runs (`internal/eval`, gold set v3) provide the patterns.

## Out of Scope

- LLM-as-reranker (RankGPT-style) — excluded by research evidence (ADR-0043); revisiting requires a new decision.
- Reranking over the dense path — the benchmark measures only GraphFusion; wiring is graph-slot only (ADR-0046).
- Model deployment topology (in-process inference vs sidecar service) — deferred to a separate decision after acceptance.
- Any change to `FusionPolicy`, `RetrievalDecision`, the orchestration gate, or `RETRIEVAL_MIN_SCORE`.
- New gold set — Gold Set v3 is used as-is.
- Score-based filtering or new thresholds on reranker output (ordering contract forbids it).
- Model pool expansion beyond the two ADR-0043 candidates.

## Further Notes

- Governed by ADR-0043 (destination & probe), ADR-0044 (seam & execution contract), ADR-0045 (evaluation & kill gate), ADR-0046 (production activation).
- Research evidence: `docs/research/rag-reranking-architectures.md` (111 inline citations; latency figures are MS MARCO-scale — the probe judges latency relatively against the ARC baseline).
- Rejection closes M8 with a closure ADR; the eval surface remains as an experiment surface only (ADR-0042 distinction: measurement tooling never shares a decision mechanism with production config).
- Glossary note: MPI/MAR introduced by this milestone — pending `/domain-modeling` glossary entry.
