# 06 — KeywordExtractorPass & CompositeEnricher Pipeline Integration

**What to build:**
Implement `KeywordExtractorPass` in `internal/pdfinspector/enrichment/keyword_pass.go`, attaching extracted keywords to both `DocumentMetadata.Keywords` and individual `KnowledgeChunk.Keywords`.

**Blocked by:** 05 — RuleBasedKeywordExtractor.

**Status:** ready-for-agent

- [ ] Implement `KeywordExtractorPass` adhering to `EnricherPass` interface seam.
- [ ] Require `CapabilityLanguage` and `CapabilitySemanticTree`, Provide `CapabilityKeywords`.
- [ ] Attach keywords hierarchically to both document-level and chunk-level.
- [ ] Integrate into `CompositeEnricher` and verify end-to-end pass execution.
