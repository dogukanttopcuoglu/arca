# 09 — DenseRetriever Implementation

**What to build:**
The `DenseRetriever` adapter in `internal/retrieval/dense` that bridges `EmbeddingProvider` and `VectorStore`, embedding query strings and performing nearest-neighbor vector search.

**Blocked by:** 02 — EmbeddingProvider Seam, 04 — InMemoryStore, 08 — Retriever Seam.

**Status:** ready-for-agent

- [ ] Implement `DenseRetriever` struct accepting `EmbeddingProvider` and `VectorStore`.
- [ ] Implement `Retrieve` method: generates query embedding, executes vector search with `MetadataFilter`, and maps points to `SearchResult` objects.
- [ ] Add unit tests for end-to-end retrieval against `InMemoryVectorStore` and `MockEmbeddingProvider`.
