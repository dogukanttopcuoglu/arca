# 01 — Domain Models, IndexSignature & MetadataFilter Abstractions

**What to build:**
Core domain types for vector indexing and metadata filtering in `internal/indexing/model`. Includes `VectorMetadata`, deterministic `IndexSignature` hash generation (`SHA256(ContentHash:Provider:Model:Version:SchemaVersion)`), and canonical `MetadataFilter` struct.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] Define `VectorMetadata` struct with document, section, and page provenance fields.
- [ ] Implement `CalculateIndexSignature` function with deterministic hashing.
- [ ] Define `MetadataFilter` struct with compile-time type-safe fields (`DocumentIDs`, `ChunkIDs`, `PageNumbers`, `ContentTypes`, `SectionPathPrefix`).
- [ ] Add unit tests verifying signature determinism and filter validation in `internal/indexing/model/model_test.go`.
