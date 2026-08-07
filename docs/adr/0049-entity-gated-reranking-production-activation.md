# Entity-gated reranking production activation (reranker-service)

The M8 global reranker was benchmark-rejected (−16..−19pp on the production distribution), but the E1/E3 research (2026-08-07) proved an **entity-gated** rerank accepts: with the BGE cross-encoder at candidate budget N=50 behind the M7 `UseGraph` gate, the entity slice gains +4.25pp nDCG with all other slices byte-identical, and the composition with the deterministic heading structure bonus is non-interfering (recorded in `docs/research/STRUCTURE_AWARE_RERANKING_RESEARCH.md`). We decided to activate reranking in production exactly as benchmarked: a provider-agnostic **reranker-service** (Python HTTP service, first model BGE-reranker-v2-m3, loaded once per service lifetime) speaking a minimal `POST /rerank` contract (`{query, candidates:[{id,text}]}` → `{ranked_ids, scores}`), consumed by an **HTTP Reranker** adapter on the existing `Reranker` seam. The runtime wraps the graph fusion retriever in `RerankedRetriever` (candidate budget N=50, ADR-0044) only when `RETRIEVAL_RERANK_URL` is set — **default-off**: with an empty URL the retrieval path is byte-identical to M9. Activation is config-gated, not code-gated; the engine is untouched (the M7 `UseGraph` decision already isolates entity queries). Fail-open is the rule: timeout/connection/5xx/invalid response degrade to the inner ordering, marked `degraded=true`, never surfaced to the user. Minimal runtime observability ships with the adapter: atomic counters (`reranker_requests_total`, `reranker_failures_total`, `reranker_latency_ms_total`) plus a structured stderr debug line per call (candidate_count, reranked_count, latency_ms, degraded, error), reported as a `Reranker:` block in the ask output. No metrics framework is added — counters are process-local and can move behind an exporter later.

Status: proposed (activation gate: production HTTP adapter must reproduce the E1 ACCEPT on artifact v3.1 via the probe's `--http-reranker-url` surface; v3.1 + v5 regression ≤ 5%; fail-open E2E with the service down; config-off byte-identical).

## Considered options

- **Reusing the probe's exec adapter in production**: rejected — ADR-0045 forbids benchmark tooling in production; a process-spawning adapter does not fit the service topology.
- **In-process Go reranker (ONNX)**: rejected — new heavyweight dependency; model swaps stay inside the service instead.
- **Global rerank activation**: rejected — M8 measured −16..−19pp on concept/comparison; only the entity-gated path has acceptance evidence.
- **Engine-level rerank option**: rejected — the M7 `UseGraph` execution-component swap already provides the gate; composition belongs to the runtime, not the engine API.
- **Metrics framework now**: rejected — no telemetry infrastructure exists; atomic counters + structured logs satisfy the comparison-with-benchmark requirement and can move behind an exporter later.

## Consequences

- `reranker-service` joins docker-compose with a GPU reservation, alongside ollama/qdrant/firecrawl.
- `RETRIEVAL_RERANK_URL` (empty = off), `RETRIEVAL_RERANK_CANDIDATE_N` (default 50, the E1-frozen value), `RETRIEVAL_RERANK_TIMEOUT_MS` (default 2000) join the runtime config; no magic numbers in code.
- The probe harness gains `--http-reranker-url` so the production adapter is benchmarked on the same artifacts and frozen gates as E1.
- The M7 `UseGraph` decision, engine, ContextBuilder, EvidenceGate, and fusion policies are untouched.
