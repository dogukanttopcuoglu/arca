# 03 — Runtime composition + config + Reranker stats block

**What to build:** The production wiring: `RETRIEVAL_RERANK_URL` (empty = off — retrieval path byte-identical to M9), `RETRIEVAL_RERANK_CANDIDATE_N` (default 50, the E1-frozen value), `RETRIEVAL_RERANK_TIMEOUT_MS` (default 2000). When the URL is set, the runtime wraps the graph fusion retriever in `RerankedRetriever` (the entity-gated path behind the M7 `UseGraph` decision — the engine is untouched). The ask output gains a `Reranker:` block (Enabled, Requests, Failures, AvgLatencyMs, Degraded) so the user can see the feature actually ran.

**Blocked by:** 02 (HTTP Reranker adapter)

**Status:** ready-for-agent

- [ ] Config defaults: URL empty → rerank off; N defaults to 50; timeout to 2000ms (no magic numbers in code)
- [ ] URL empty → retrieval behavior byte-identical to M9 (wrapper passthrough; verified by composition test and E2E)
- [ ] URL set → the graph fusion retriever is wrapped with RerankedRetriever(N=50, HTTP adapter); entity queries execute through it; non-entity paths untouched (the engine's UseGraph decision is the only gate)
- [ ] Adapter construction failure (invalid URL, unreachable at startup) degrades to off with a warning — never a hard failure
- [ ] Ask output renders the `Reranker:` block: Enabled true/false, Requests, Failures, AvgLatencyMs, Degraded (last call)
- [ ] Fail-open confirmed at the composition level: service down → entity question answered from the inner ordering, no user-visible error
