# 02 — HTTP Reranker adapter + minimal observability

**What to build:** The production `Reranker` seam adapter in Go: an HTTP client that depends only on the `POST /rerank` contract and maps `{ranked_ids, scores}` to the seam's `[]ScoredCandidate`. Every failure mode degrades identically to the inner retriever ordering (`degraded=true`, `Stats.RerankerFailed`); the adapter carries the minimal runtime observability — atomic counters (`reranker_requests_total`, `reranker_failures_total`, `reranker_latency_ms_total`) and one structured stderr debug line per call (candidate_count, reranked_count, latency_ms, degraded, error).

**Blocked by:** None — contract-first development with `httptest`; the service (ticket 01) is not required for unit tests.

**Status:** ready-for-agent

- [ ] Adapter implements the existing `Reranker` seam; ranked_ids order maps to ScoredCandidate order with aligned scores; `top_k` is never sent (the wrapper truncates)
- [ ] Fail-open test matrix — all four failure classes produce the identical degrade behavior (error surfaced to the wrapper's graceful degradation, no panic, no partial output):
  - [ ] timeout (context deadline)
  - [ ] connection refused
  - [ ] 5xx response
  - [ ] invalid response (bad JSON, missing fields, length mismatch)
- [ ] Counters increment correctly across happy/failure paths; latency total accumulates per call
- [ ] One structured debug log line per call carries candidate_count, reranked_count, latency_ms, degraded, and error (when present)
- [ ] The ordering contract is preserved: the wrapper's `StabilizeOrdering` remains the single tie-break enforcement point (adapter never re-orders beyond ranked_ids)
