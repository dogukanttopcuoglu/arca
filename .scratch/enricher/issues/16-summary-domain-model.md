# 16 — Summary Domain Model & Capability Token

**What to build:**
Add `internal/pdfinspector/model/summary.go` with `Summary` and `SummarySource` structs. Add `Summary *Summary` field to `DocumentMetadata` and `KnowledgeChunk`. Declare `CapabilitySummary` token in `internal/pdfinspector/enrichment/pass.go`.

**Blocked by:** None.

**Status:** ready-for-agent

- [ ] Create `internal/pdfinspector/model/summary.go` with `Summary` and `SummarySource`.
- [ ] Add `Summary *Summary` to `DocumentMetadata` and `KnowledgeChunk`.
- [ ] Declare `CapabilitySummary` token in `pass.go`.
