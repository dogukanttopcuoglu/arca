---
title: 05 — MinScore threshold and no_evidence behavior
feature: m3-retrieval-quality
status: ready-for-agent
created: 2026-08-03
blocked-by: None
---

# 05 — MinScore threshold / no_evidence behavior

**What to build:** retrieval currently has no effective score floor, so on a populated index every query returns top-k neighbors and the `no_evidence` path is unreachable — the LLM is called even for irrelevant queries. Wire a configurable `MinScore` (composition config, default 0) through `arc ask`/`arc eval`, and make abstention fire whenever no results survive the threshold. Mode-aware: in hybrid mode, thresholds apply per-stream before RRF fusion; an empty fused result abstains.

**Blocked by:** None — can start immediately (hybrid interplay lands with 08).

**Status:** ready-for-agent

- [ ] Retrieval threshold is composition config with a sane default; `arc ask` passes it through to the retriever
- [ ] An Abstention Query with no results above threshold yields `no_evidence` and never invokes the LLM (spy-asserted at the AnswerEngine seam)
- [ ] **Calibrated operating point (REVISED per benchmark, docs/benchmarks/calibration_min_score_v1.json):** `RETRIEVAL_MIN_SCORE=0.6` is the M3 calibrated default — highest threshold with recall@5 flat vs the dense baseline (within the 5% regression budget) while raising no_evidence_precision 0 → 0.375
- [ ] **Acceptance criterion REVISED:** the previous requirement of `no_evidence_precision == 1.0` is removed — the calibration sweep showed a single global cosine threshold cannot maximize recall and abstention simultaneously on a semantically coherent corpus (at 0.7 abstention reaches 1.0 but recall regresses 21% beyond budget). M3 establishes the calibrated baseline; high-precision abstention is deferred to M4 where richer relevance signals (hybrid retrieval, sparse evidence, reranking, semantic relevance gating) become available
- [ ] Threshold applies per-stream in hybrid mode; empty fused results abstain
- [ ] Existing retrieval contracts (dense cosine semantics, filters) unchanged
