# 02 — RerankedRetriever wrapper

**What to build:** The `RerankedRetriever` execution component (ADR-0044) implementing the existing `seam.Retriever`: it requests candidate budget N from the inner retriever, calls the `Reranker`, truncates to the caller's TopK, orders deterministically, and handles errors — a reranker failure degrades gracefully to the inner retriever's result with a diagnostic. From the outside it behaves like a standard retriever (caller asks TopK=K, gets K); candidate budget is wrapper-internal. It never filters and never writes scores back into fusion.

**Blocked by:** 01 — Reranker seam and ordering contract.

**Status:** resolved

- [ ] Wrapper implements `seam.Retriever` with identical external semantics (K in, K out)
- [ ] Requests N candidates internally regardless of the caller's K
- [ ] Deterministic final ordering including ChunkID ASC tie-break
- [ ] Reranker failure falls back to inner retriever result with diagnostic (Graceful Degradation)
- [ ] Empty candidate list stays empty — abstention behavior preserved (hard invariant)
- [ ] No filtering and no write-back to fusion (ordering contract)
- [ ] Tests through the `seam.Retriever` surface with fake inner retriever + fake reranker

## Comments
