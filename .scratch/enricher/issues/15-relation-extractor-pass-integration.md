# 15 — RelationExtractorPass & CompositeEnricher Pipeline Integration

**What to build:**
Implement `RelationExtractorPass` in `internal/pdfinspector/enrichment/relation_pass.go`, attaching relations to document metadata (canonical catalog) and chunks.

**Blocked by:** 14 — RelationExtractor Interface & RuleBasedRelationExtractor.

**Status:** ready-for-agent

- [ ] Implement `RelationExtractorPass` adhering to `EnricherPass` interface seam.
- [ ] Require `CapabilityEntities`, `CapabilityConcepts`, Provide `CapabilityRelations`.
- [ ] Populate `DocumentMetadata.Relations` with a canonical, deduplicated relation catalog.
- [ ] Attach chunk-specific `Relation` slices to `KnowledgeChunk.Relations`.
- [ ] Integrate into `CompositeEnricher` and add unit tests verifying end-to-end pipeline execution.
