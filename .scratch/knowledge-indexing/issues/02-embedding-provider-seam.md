# 02 — EmbeddingProvider Interface Seam & Mock Adapter

**What to build:**
The minimal `EmbeddingProvider` interface seam (`GenerateEmbeddings`, `Capabilities`, `Health`), `ProviderCapabilities` encapsulation, `EmbeddingResult` type, and `MockEmbeddingProvider` adapter for zero-network testing in `internal/indexing/provider`.

**Blocked by:** 01 — Domain Models, IndexSignature & MetadataFilter Abstractions.

**Status:** ready-for-agent

- [ ] Define `EmbeddingProvider` interface seam and `ProviderCapabilities` struct.
- [ ] Define `EmbeddingResult` containing vectors, token usage, provider, model, and version metadata.
- [ ] Implement `MockEmbeddingProvider` generating deterministic vectors (e.g. 1536d) for input texts.
- [ ] Add unit tests for mock embedding generation, capability checks, and health pinging.
