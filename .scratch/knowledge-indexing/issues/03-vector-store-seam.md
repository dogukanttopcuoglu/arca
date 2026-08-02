# 03 — VectorStore Interface Seam & VectorPoint Contract

**What to build:**
The low-level `VectorStore` persistence interface (`UpsertPoints`, `SearchVector`, `Delete`, `Health`), `VectorPoint` contract, and constructor-bound collection scoping in `internal/indexing/store`.

**Blocked by:** 01 — Domain Models, IndexSignature & MetadataFilter Abstractions.

**Status:** ready-for-agent

- [ ] Define `VectorPoint` struct containing stable Point ID, float32 vector, and `VectorMetadata`.
- [ ] Define `VectorStore` interface seam with constructor-bound collection scope.
- [ ] Implement `CalculatePointID(docID, sectionPath string, chunkOrder int)` helper.
- [ ] Add unit tests verifying Point ID stability across content revisions.
