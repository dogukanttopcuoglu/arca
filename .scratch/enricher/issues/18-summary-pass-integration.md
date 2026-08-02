# 18 — SummaryPass & CompositeEnricher Pipeline Integration

**What to build:**
Implement `SummaryPass` in `internal/pdfinspector/enrichment/summary_pass.go`, attaching extracted `SummaryResult` to `DocumentMetadata.Summary` and `KnowledgeChunk.Summary`.

**Blocked by:** 17 — SummaryExtractor Interface & Extractive RuleBasedSummaryExtractor.

**Status:** ready-for-agent

- [ ] Implement `SummaryPass` adhering to `EnricherPass` interface seam.
- [ ] Require `CapabilityKeywords`, `CapabilityEntities`, `CapabilityConcepts`, `CapabilityRelations`, Provide `CapabilitySummary`.
- [ ] Attach `DocumentSummary` to `DocumentMetadata.Summary` and chunk summaries to `KnowledgeChunk.Summary`.
- [ ] Integrate into `CompositeEnricher` and add unit tests verifying end-to-end pipeline execution.
