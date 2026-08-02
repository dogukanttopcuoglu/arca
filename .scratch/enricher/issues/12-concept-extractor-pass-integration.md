# 12 — ConceptExtractorPass & CompositeEnricher Pipeline Integration

**What to build:**
Implement `ConceptExtractorPass` in `internal/pdfinspector/enrichment/concept_pass.go`, attaching concepts to document metadata and chunks.

**Blocked by:** 11 — ConceptExtractor Interface & RuleBasedConceptExtractor.

**Status:** ready-for-agent

- [ ] Implement `ConceptExtractorPass` adhering to `EnricherPass` interface seam.
- [ ] Require `CapabilityLanguage`, `CapabilityKeywords`, `CapabilityEntities`, Provide `CapabilityConcepts`.
- [ ] Attach `Concept` slice to document metadata and individual chunks.
- [ ] Integrate into `CompositeEnricher` and add unit tests verifying end-to-end pipeline execution.
