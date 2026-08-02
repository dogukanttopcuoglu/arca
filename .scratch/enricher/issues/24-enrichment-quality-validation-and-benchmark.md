# 24 — Enrichment Quality Validation & Empirical Benchmark

**Problem Statement:**
Empirical quality improvements (elimination of unigram concept leaks, correction of relation predicate directionality) need automated contract validation and regression benchmark tests.

**Solution:**
Implement integration benchmark tests in `internal/pdfinspector/enrichment/quality_benchmark_test.go` verifying that no unigrams populate `Metadata.Concepts` and relation predicate directions are strictly valid.

**Commits:**
1. `test(enrichment): add empirical quality benchmark and relation direction contract tests`

**Blocked by:**
- #21 (Rule-Based Extractors Domain Boundary Migration)
- #22 (Composite Enricher Dependency Injection)

**Out of Scope:**
- Codebase changes.
