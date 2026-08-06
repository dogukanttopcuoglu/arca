# 03 — Candidate artifact generation

**What to build:** The eval harness (ADR-0027) records candidate lists as benchmark artifacts per ADR-0045's normative format: benchmark fingerprint, query id, ordered candidate ids, baseline retrieval metadata, candidate scores (informational only), reranker output ordering. Generation is deterministic — the same corpus fingerprint and gold set version produce the same artifact — so different rerankers can be re-run over identical candidates later and retrieval variance is fully eliminated. Runs against the unchanged production baseline (GraphFusionRetriever, w=1.0, K=5).

**Blocked by:** None — can start immediately.

**Status:** resolved

- [ ] Artifact generation runs against the unchanged production baseline (single-variable design)
- [ ] Artifact contains all ADR-0045 fields (fingerprint, query id, ordered candidate ids, baseline metadata, scores info-only, reranker output ordering)
- [ ] Deterministic: same fingerprint + gold set -> identical artifact (proven by re-run)
- [ ] Abstention queries recorded with empty candidate lists
- [ ] Artifacts stored per benchmark run for re-runnable reranker comparisons

## Comments
