# 08 — EntityExtractor Interface & RuleBasedEntityExtractor

**What to build:**
Implement `EntityExtractor` interface seam returning `[]EntityMention` and deterministic `RuleBasedEntityExtractor` in `internal/pdfinspector/enrichment/entity_extractor.go`.

**Blocked by:** 07 — Entity Domain Models & Capability Token.

**Status:** ready-for-agent

- [ ] Define `EntityExtractor` interface seam (`ExtractEntities(ctx, chunks, lang) ([]EntityMention, error)`).
- [ ] Implement `RuleBasedEntityExtractor` with regex/dictionary patterns for Person, Organization, Location, Product.
- [ ] Ensure confidence scores and chunk_id provenance are correctly populated on `EntityMention` records.
- [ ] Add unit tests verifying empty docs, single chunk, multi-chunk, TR/EN support, and deterministic extraction.
