# 07 — Asynchronous Indexing Worker & Provider Batching Engine

**What to build:**
The `IndexingWorker` in `internal/indexing/worker` orchestrating `DiffEngine`, `EmbeddingProvider`, and `VectorStore`, supporting auto-batching and synchronous/asynchronous job execution.

**Blocked by:** 02 — EmbeddingProvider Seam, 04 — InMemoryStore, 05 — Diff Engine, 06 — Job Model.

**Status:** ready-for-agent

- [ ] Implement `IndexingWorker` struct supporting both `ExecuteSync` and async background execution.
- [ ] Implement provider batch limit enforcement using `ProviderCapabilities.MaxBatchSize`.
- [ ] Implement diff plan execution: skip unchanged chunks, generate embeddings for modified/new chunks, upsert to vector store, and update job status.
- [ ] Add unit tests verifying end-to-end sync indexing, differential skipping, and provider error handling.
