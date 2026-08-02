# 10 — Hybrid Retrieval Foundation & Reciprocal Rank Fusion (RRF)

**What to build:**
The architectural scaffolding for Hybrid Retrieval in `internal/retrieval/hybrid`, featuring Reciprocal Rank Fusion (RRF) score normalization, composite `HybridRetriever`, and testing mocks.

**Blocked by:** 08 — Retriever Seam, 09 — DenseRetriever Implementation.

**Status:** ready-for-agent

- [ ] Implement `ScoreFusionEngine` and Reciprocal Rank Fusion (RRF) formula ($k=60$).
- [ ] Implement `HybridRetriever` struct combining multiple sub-retriever streams.
- [ ] Implement `MockSparseRetriever` for testing hybrid fusion mechanics.
- [ ] Add unit tests verifying RRF score merging, rank ordering, and disjoint result sets.
