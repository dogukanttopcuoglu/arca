---
title: 00 — M10 spec: entity-gated reranking production activation
feature: m10-reranking-activation
status: ready-for-agent
created: 2026-08-07
blocked-by: None — decisions frozen in ADR-0049; grilling closed (Q1-Q4)
---

# M10 Reranking Activation Spec

## Problem Statement

The M8 reranker was rejected globally, but E1/E3 (2026-08-07) proved the entity-gated path accepts: BGE cross-encoder at N=50 behind the M7 `UseGraph` gate gains +4.25pp on the entity slice with all other slices byte-identical. The acceptance was measured on the **probe surface** (an exec adapter that is forbidden in production by ADR-0045). The benchmarked capability is not wired into the product: entity questions still use raw GraphFusion.

## Solution

Ship the accepted capability through production surfaces: a provider-agnostic **reranker-service** (Python HTTP, first model BGE), an **HTTP Reranker** adapter on the existing `Reranker` seam, and runtime composition that wraps the graph fusion retriever in `RerankedRetriever` (N=50) **only when `RETRIEVAL_RERANK_URL` is set** (default-off — empty URL keeps M9 behavior byte-identical). Fail-open everywhere; minimal observability (atomic counters + structured debug log + a `Reranker:` block in ask output). Activation is config-gated and benchmark-gated: the production HTTP adapter must reproduce the E1 ACCEPT.

## User Stories

1. As an operator, I want `RETRIEVAL_RERANK_URL` empty to leave the retrieval path byte-identical, so that the default deployment never changes behavior.
2. As an operator, I want to enable reranking by setting one URL, so that activation is a config change, not a code change.
3. As a user asking an entity question ("What does the book say about World Bank?"), I want the answer to use the reranked top-5 so that the benchmarked +4.25pp entity gain reaches the product.
4. As a user asking concept/comparison/section questions, I want reranking to never touch my path so that the M8 loss classes stay untouched.
5. As a user, I want the reranker service to go down without breaking answers — retrieval falls back to GraphFusion ordering, no error surfaced.
6. As a user, I want the ask output to show a `Reranker:` block (Enabled/Requests/Failures/AvgLatencyMs/Degraded) so that I can see the feature actually ran.
7. As an operator, I want per-call structured debug logs (candidate_count, reranked_count, latency_ms, degraded, error) so that production usage is comparable with the benchmark.
8. As an engineer, I want to swap the reranker model (BGE → Jina/Qwen/Cohere) inside the service without touching the Go side, so that model choice stays provider-agnostic.
9. As a benchmark maintainer, I want the probe to measure the production HTTP adapter on the same artifact and gates as E1, so that acceptance is reproduced on the real surface.
10. As an operator, I want `RETRIEVAL_RERANK_CANDIDATE_N` (default 50) and `RETRIEVAL_RERANK_TIMEOUT_MS` (default 2000) as the only other knobs, so that the config surface stays minimal.

## Implementation Decisions

1. **reranker-service (new service).** Python HTTP service (`services/reranker-service/`): FastAPI, model loaded once at startup (BGE-reranker-v2-m3 via sentence-transformers CrossEncoder), GPU. Single endpoint `POST /rerank`:
   - Request: `{"query": "...", "candidates": [{"id": "...", "text": "..."}], "top_k": N?}` (`top_k` optional, unused by the Go adapter).
   - Response: `{"ranked_ids": ["..."], "scores": [0.9, ...]}` — ranked_ids order is the contract; scores are informational.
   - Docker: joins docker-compose with `deploy.devices` GPU reservation (the ollama pattern), healthcheck, model downloaded on first start.
   - The probe's `bge_rerank.py` is the seed; the service keeps the same model-inference logic (small batches, per-request cache eviction — the WSL/CUDA stability notes).

2. **HTTP Reranker (new adapter, `internal/retrieval/rerank`).** Implements the existing `Reranker` seam. Depends only on the HTTP contract: `POST /rerank` with query + candidates `{id, text}` → `ranked_ids`/`scores` mapped to `[]ScoredCandidate` (ranked_ids order preserved, scores aligned). `top_k` is not sent; the wrapper truncates and `StabilizeOrdering` enforces the tie-break contract.

3. **Fail-open.** Timeout / connection error / 5xx / invalid response (bad JSON, missing fields, wrong lengths, non-200) → error → `RerankedRetriever`'s existing graceful degradation returns the inner ordering, `Stats.RerankerFailed` set, `degraded=true` in the log. No retry, single call, timeout covers only the HTTP call.

4. **Observability (minimal).** Atomic counters on the adapter: `reranker_requests_total`, `reranker_failures_total`, `reranker_latency_ms_total` (sum). One structured stderr debug line per call: `candidate_count`, `reranked_count`, `latency_ms`, `degraded`, `error` (when present). The ask output gains a `Reranker:` block: `Enabled true/false`, `Requests`, `Failures`, `AvgLatencyMs`, `Degraded` (last call). No metrics framework.

5. **Runtime composition (default-off).** New config keys: `RETRIEVAL_RERANK_URL` (empty = off), `RETRIEVAL_RERANK_CANDIDATE_N` (default 50 — the E1-frozen value, no magic numbers), `RETRIEVAL_RERANK_TIMEOUT_MS` (default 2000). In `buildAnswerEngine`, when URL is non-empty, the graph fusion retriever is wrapped: `RerankedRetriever(graphFusion, {N, httpReranker})`. The engine, `UseGraph` decision, ContextBuilder, EvidenceGate, fusion policies are untouched.

6. **Probe surface (`--http-reranker-url`).** The probe run CLI gains a flag to use the production HTTP adapter instead of the exec adapter, so the same artifact + frozen gates measure the production surface. Benchmark tooling only.

## Testing Decisions

- **What makes a good test:** external behavior — the adapter's contract mapping and fail-open paths (via `httptest`), the runtime composition (byte-identical when off, wrapped when on), the probe flag, the Stats block rendering. No Python implementation details.
- **rerank package:** HTTP adapter tests with `httptest.Server`: happy path (ranked_ids → ScoredCandidate order), 500 → error (degraded), timeout → error, invalid JSON → error, malformed response (missing fields, length mismatch) → error; counter assertions; debug log emission. Prior art: `rerank_test.go` (existing wrapper tests).
- **CLI:** runtime config defaults (URL empty → off), composition smoke (URL set → wrapped retriever), Stats block rendering in ask output (Enabled false when off; true + counters when on with a fake HTTP reranker). Prior art: `runtime_test.go`, `app_test.go`.
- **Probe:** `--http-reranker-url` flag path (probe run with the adapter against a fake server). Prior art: `probe_test.go`.
- **Service:** smoke-tested in validation (container up, `/rerank` returns a valid response for a canned pair); no Go unit tests for Python.

## Out of Scope

- Global reranking (M8-rejected), heading-intent detector (deferred second leg), exemplar-based intent matcher (Phase 2), metrics framework/Prometheus, retries, Ollama-native rerank, engine changes, new gold sets (v3.1/v5 unchanged), E5 payload extension.

## Further Notes

- Activation gate (frozen): (1) probe `--http-reranker-url` on artifact_v3_1 reproduces E1 ACCEPT (N=50, p95 ≤ 8s, RSS ≤ 4GiB, entity slice +4.25pp); (2) v3.1 + v5 regression ≤ 5%; (3) fail-open E2E (service down → answer with `degraded=true`, no user-visible error); (4) config-off byte-identical (empty URL); (5) observability verified (counters + structured log + Stats block).
- ADR-0049 records the decision (proposed; flipped to accepted by the gate).
- Milestone naming: M10 (M8 = reranking research/rejection; M9 = document-level retrieval).
