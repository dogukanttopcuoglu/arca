# 08 — High-Level Retriever Seam & Search Contracts

**What to build:**
The clean `Retriever` interface seam (`Retrieve(ctx, query)`), `RetrievalMode` enum (`Dense`, `Sparse`, `Hybrid`), `RetrievalQuery`, and `SearchResult` domain types in `internal/retrieval/seam`.

**Blocked by:** 01 — Domain Models, IndexSignature & MetadataFilter Abstractions.

**Status:** ready-for-agent

- [ ] Define `RetrievalMode` enum and `RetrievalQuery` struct.
- [ ] Define `SearchResult` struct isolating downstream RAG consumers from vector database types.
- [ ] Define `Retriever` interface seam.
- [ ] Add unit tests for query validation and search result sorting helpers.
