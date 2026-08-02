# 07 — Entity Domain Models & Capability Token

**What to build:**
Add `EntityType`, `EntityMention`, and `Entity` structs to `internal/pdfinspector/model/model.go`. Add `Entities` fields to `DocumentMetadata` and `KnowledgeChunk`. Declare `CapabilityEntities` token in `internal/pdfinspector/enrichment/pass.go`.

**Blocked by:** None.

**Status:** ready-for-agent

- [ ] Add `EntityType` enum (`person`, `organization`, `location`, `product`, `event`, `miscellaneous`).
- [ ] Add `EntityMention` struct (`Text`, `Type`, `ChunkID`, `Confidence`).
- [ ] Add `Entity` struct (`ID`, `Name`, `Type`, `Aliases`, `Mentions`, `Score`).
- [ ] Update `DocumentMetadata`: Add `Entities []Entity` field.
- [ ] Update `KnowledgeChunk`: Add `Entities []EntityMention` field.
- [ ] Add `CapabilityEntities` token to `pass.go`.
