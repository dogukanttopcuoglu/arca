# 14 — RelationExtractor Interface & RuleBasedRelationExtractor

**What to build:**
Implement `RelationExtractor` interface seam and deterministic `RuleBasedRelationExtractor` in `internal/pdfinspector/enrichment/relation_extractor.go`.

**Blocked by:** 13 — Relation Domain Model & Capability Token.

**Status:** ready-for-agent

- [ ] Define `RelationExtractor` interface seam (`ExtractRelations(ctx, input) ([]Relation, error)`).
- [ ] Implement `RuleBasedRelationExtractor` extracting `Entity ↔ Entity` and `Entity ↔ Concept` relations based on co-occurrence and predicate patterns.
- [ ] Ensure deterministic relation IDs (`rel:subjectID:predicate:objectID`) for stable deduplication.
- [ ] Add unit tests verifying empty input, SPO extraction, confidence scoring, and ID stability.
