# 06 — IndexingJob Model & State Machine

**What to build:**
The `IndexingJob` model and explicit status state machine (`Pending`, `Running`, `Completed`, `Failed`, `Retrying`, `Cancelled`) with progress metrics in `internal/indexing/job`.

**Blocked by:** 01 — Domain Models, IndexSignature & MetadataFilter Abstractions.

**Status:** ready-for-agent

- [ ] Define `IndexingStatus` enum and `IndexingJob` struct.
- [ ] Implement valid state transition rules and invalid transition error checking.
- [ ] Add progress tracking helpers (`IndexedChunks`, `TotalChunks`, `SkippedChunks`).
- [ ] Add unit tests for valid/invalid state transitions and progress calculations.
