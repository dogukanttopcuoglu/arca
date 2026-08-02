# 09 — EntityExtractorPass & CompositeEnricher Pipeline Integration

**What to build:**
Implement `EntityExtractorPass` in `internal/pdfinspector/enrichment/entity_pass.go`, attaching mentions to `KnowledgeChunk.Entities` and performing minimal in-memory mention grouping for `DocumentMetadata.Entities`.

**Blocked by:** 08 — EntityExtractor Interface & RuleBasedEntityExtractor.

**Status:** ready-for-agent

- [ ] Implement `EntityExtractorPass` adhering to `EnricherPass` interface seam.
- [ ] Require `CapabilityLanguage` and `CapabilitySemanticTree`, Provide `CapabilityEntities`.
- [ ] Attach `EntityMention` slice to individual `KnowledgeChunk.Entities`.
- [ ] Perform minimal in-memory grouping to populate `DocumentMetadata.Entities`.
- [ ] Integrate into `CompositeEnricher` and add unit tests verifying end-to-end pipeline execution.
