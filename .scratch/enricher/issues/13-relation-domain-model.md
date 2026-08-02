# 13 — Relation Domain Model & Capability Token

**What to build:**
Add `internal/pdfinspector/model/relation.go` with `Relation`, `RelationType`, and `RelationSource` structs. Add `Relations []Relation` field to `DocumentMetadata` and `KnowledgeChunk`. Declare `CapabilityRelations` token in `internal/pdfinspector/enrichment/pass.go`.

**Blocked by:** None.

**Status:** ready-for-agent

- [ ] Create `internal/pdfinspector/model/relation.go` with `Relation`, `RelationType`, and `RelationSource`.
- [ ] Add `Relations []Relation` to `DocumentMetadata` and `KnowledgeChunk`.
- [ ] Declare `CapabilityRelations` token in `pass.go`.
