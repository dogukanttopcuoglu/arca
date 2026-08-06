# 07 — Production activation: wiring and config

**What to build:** Conditional on acceptance (ticket 06): wire `NewRerankedRetriever(graphFusionRetriever, rerankConfig)` into the graph slot of the AnswerEngine composition (ADR-0046) — the M7 slot structure, orchestration gate, and dense path stay untouched. Introduce `RETRIEVAL_RERANK_MODEL` (empty = feature off, no separate enabled flag) and `RETRIEVAL_RERANK_CANDIDATE_N` (wrapper-internal). Production config must match the benchmark-accepted fingerprint one-to-one (model, N) — a different value invalidates the acceptance. Rollback is a single config change. If ticket 06 rejected, this ticket is cancelled: no config keys, no wiring, eval surface remains experiment-only.

**Blocked by:** 06 — Probe run and freeze/closure decision.

**Status:** ready-for-agent

- [ ] Wrapper wired into the graph slot by composition only — engine concept unchanged
- [ ] `RETRIEVAL_RERANK_MODEL=""` == feature off (no ENABLED flag)
- [ ] `RETRIEVAL_RERANK_CANDIDATE_N` controls only wrapper-internal budget; TopK semantics unchanged
- [ ] Production config equals the benchmark-accepted (model, N) — fingerprint locked
- [ ] Rollback proven: emptying the model value restores M7 behavior (single config change, no code)
- [ ] Dense path and `FusionPolicy` untouched

## Comments
