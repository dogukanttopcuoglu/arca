# 05 — Differential Re-Indexing & Diff Matrix Engine

**What to build:**
The `DiffEngine` in `internal/indexing/diff` that compares incoming document `KnowledgeChunk` signatures against stored `IndexSignature`s, generating an actionable `DiffPlan` (`UNCHANGED`, `CONTENT_CHANGED`, `MODEL_CHANGED`, `NEW`, `DELETED`).

**Blocked by:** 01 — Domain Models, 03 — VectorStore Interface Seam.

**Status:** ready-for-agent

- [ ] Implement `DiffEngine` and `DiffPlan` structures.
- [ ] Implement 6-state signature diff classification logic.
- [ ] Implement detection for unchanged chunks (skip API calls), changed chunks (re-embed & inplace upsert), and removed chunks (delete).
- [ ] Add unit tests for zero-work re-indexing, modified section updates, and model version changes.
