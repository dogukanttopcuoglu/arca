# 21 — Rule-Based Extractors Domain Boundary Migration

**Problem Statement:**
`RuleBasedConceptExtractor` allows unigram location fragments (`concept:york`) to leak into concepts, and `RuleBasedRelationExtractor` produces inverted predicate directions (`Rick Rubin -> founded_by -> Def Jam`).

**Solution:**
Update `RuleBasedConceptExtractor` to filter out single-word unigrams and location fragments. Update `RuleBasedRelationExtractor` to enforce predicate directionality (`Def Jam -> founded_by -> Rick Rubin` or `Rick Rubin -> founded -> Def Jam`).

**Commits:**
1. `fix(enrichment): filter unigrams and location fragments in RuleBasedConceptExtractor`
2. `fix(enrichment): validate predicate directionality in RuleBasedRelationExtractor`

**Blocked by:**
- #20 (Provider Interfaces & Domain Boundaries)

**Out of Scope:**
- GLiNER HTTP client integration.
