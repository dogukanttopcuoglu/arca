# 10 — Concept Domain Model & Capability Token

**What to build:**
Add `internal/pdfinspector/model/concept.go` with `Concept` and `ConceptSource` structs. Add `Concepts []Concept` field to `DocumentMetadata` and `KnowledgeChunk`. Declare `CapabilityConcepts` token in `internal/pdfinspector/enrichment/pass.go`.

**Blocked by:** None.

**Status:** ready-for-agent

- [ ] Create `internal/pdfinspector/model/concept.go` with `Concept` and `ConceptSource`.
- [ ] Add `Concepts []Concept` to `DocumentMetadata` and `KnowledgeChunk`.
- [ ] Declare `CapabilityConcepts` token in `pass.go`.
