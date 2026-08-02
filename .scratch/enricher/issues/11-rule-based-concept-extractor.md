# 11 — ConceptExtractor Interface & RuleBasedConceptExtractor

**What to build:**
Implement `ConceptExtractor` interface seam and deterministic `RuleBasedConceptExtractor` in `internal/pdfinspector/enrichment/concept_extractor.go`.

**Blocked by:** 10 — Concept Domain Model & Capability Token.

**Status:** ready-for-agent

- [ ] Define `ConceptExtractor` interface seam (`ExtractConcepts(ctx, input, lang) ([]Concept, error)`).
- [ ] Implement `RuleBasedConceptExtractor` synthesizing section headings and key phrases into topics.
- [ ] Add unit tests verifying empty input, heading synthesis, score ranking, and deterministic output.
