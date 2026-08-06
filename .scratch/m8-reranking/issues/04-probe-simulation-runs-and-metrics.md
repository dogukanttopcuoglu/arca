# 04 — Probe simulation runs and metrics

**What to build:** The probe harness runs all 3 combinations (cross-encoder BGE-reranker-v2-m3 at N = 20/50/100) over the recorded candidate artifacts from ticket 03, using the `RerankedRetriever` from ticket 02. Every combination reports: nDCG@5, MRR (binary relevance from gold set v3), answer quality / EvidenceGate behavior, p50/p95 latency, memory footprint, cold vs warm model load time. Latency is reported relatively against the production baseline — absolute figures are informative only (small corpus).

**Blocked by:** 02 — RerankedRetriever wrapper; 03 — Candidate artifact generation.

**Status:** resolved

- [ ] All 3 combinations run over the same recorded artifacts (retrieval variance eliminated)
- [ ] Per-combination metrics: nDCG@5, MRR, answer quality, p50/p95, memory, cold/warm load
- [ ] Metrics computed with binary relevance per ADR-0045 (matching M7 calibration)
- [ ] Answer quality measured on every combination (not just ranking)
- [ ] Relative latency reported against production baseline
- [ ] Rerank determinism holds per run (same artifact -> same ordering)

## Comments
