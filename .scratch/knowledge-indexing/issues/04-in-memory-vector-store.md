# 04 — InMemoryVectorStore Implementation

**What to build:**
A zero-dependency, thread-safe `InMemoryVectorStore` adapter implementing exact Cosine Similarity vector search, inplace point upserts, and `MetadataFilter` matching in `internal/indexing/store/inmemory.go`.

**Blocked by:** 03 — VectorStore Interface Seam & VectorPoint Contract.

**Status:** ready-for-agent

- [ ] Implement `InMemoryVectorStore` with `sync.RWMutex` thread safety.
- [ ] Implement exact Cosine Similarity vector distance ranking.
- [ ] Implement `MetadataFilter` matching engine for in-memory points.
- [ ] Add unit tests verifying cosine ordering, inplace upserts, filter matching, and race conditions (`go test -race`).
