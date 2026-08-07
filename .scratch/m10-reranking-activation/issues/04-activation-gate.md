# 04 — Probe --http-reranker-url + activation gate

**What to build:** The M10 acceptance evidence: the probe run gains `--http-reranker-url` so the production HTTP adapter is measured on the same artifact and frozen gates as E1, and the full activation gate runs against the live service — E1 ACCEPT reproduced, regressions clean, fail-open proven, observability verified. ADR-0049 flips to accepted with the M10 validation report.

**Blocked by:** 01 (reranker-service), 02 (HTTP Reranker adapter), 03 (runtime composition)

**Status:** ready-for-agent

- [ ] Probe run accepts `--http-reranker-url` and measures the production adapter on artifact_v3_1 with the same frozen budgets and gates as E1
- [ ] Gate 1 — E1 ACCEPT reproduced on the production surface: N=50, entity slice +4.25pp, p95 ≤ 8s, RSS ≤ 4GiB, verified ≥ baseline −1pp
- [ ] Gate 2 — regression: v3.1 and v5 re-runs within 5% (non-entity slices untouched by construction, measured anyway)
- [ ] Gate 3 — fail-open E2E: reranker-service stopped → entity question answered from GraphFusion ordering with `degraded=true` in the log, no user-visible error
- [ ] Gate 4 — config-off byte-identical: empty `RETRIEVAL_RERANK_URL` produces the M9 behavior exactly
- [ ] Gate 5 — observability verified: counters increment, structured debug log lines parse, ask output shows the `Reranker:` block with correct values
- [ ] ADR-0049 flipped to accepted + `docs/benchmarks/M10_VALIDATION.md` recorded
