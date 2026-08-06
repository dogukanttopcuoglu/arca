# M8 Reranking — Closeout: benchmark-rejected (production unchanged)

Status: closed
Date: 2026-08-06
Gate: ADR-0045 kill gate — REJECT on Gold Set v3 (MPI/MAR violated); v4 heading slice gained but cannot override v3 regression or the abstention invariant.

## Decision

**Reranking is not activated.** The BGE-reranker-v2-m3 second-stage reranker failed the frozen acceptance thresholds on the production query distribution. `RETRIEVAL_RERANK_MODEL` / `RETRIEVAL_RERANK_CANDIDATE_N` are not introduced; the `RerankedRetriever` wrapper and probe tooling remain as experiment surface only (ADR-0042 distinction). Production behavior is byte-identical to M7.

## Benchmark setup (valid measurement)

- Environment: Ollama on NVIDIA GPU (100% GPU, verified), Qdrant live corpus (3017 chunks, fingerprint `8b21a664…` — gold set v3 declared, verified before any query).
- Content path verified payload-first (probe content from vector store payload — the earlier M8 warm-up ran against empty process-local content and is superseded by this run).
- Gold sets: v3 (37 queries, regression canary) and v4 (14 heading queries, additive slice).
- Candidate artifact: GraphFusionRetriever w=1.0, N=100 (`/tmp/artifact_v3.json`, `/tmp/artifact_v4.json`).
- Budgets frozen after warm-up: p95 ≤ 8000 ms, RSS ≤ 4 GiB (measured 3463-3490 MB).
- Two probe-side defects found and fixed during the run (committed `9fb2f0a`): evidence gate `max_tokens` 512→2048 (DeepSeek reasoning consumed the completion budget on long contexts — verified by direct API probes, `finish_reason=length`, empty content) and probe gate input capped at the engine context budget (LLM_CONTEXT_BUDGET, default 4000).

## Results

### Gold Set v3 (production query distribution)

| configuration | recall@5 | nDCG@5 | MRR | verified | p95 ms | RSS MB |
|---|---|---|---|---|---|---|
| Baseline (GraphFusion w=1.0) | **0.956** | **0.886** | **0.902** | 0.655 | — | — |
| BGE N=20 | 0.737 | 0.724 | 0.761 | 0.759 | 2293 | 3463 |
| BGE N=50 | 0.694 | 0.703 | 0.753 | 0.793 | 4132 | 3463 |
| BGE N=100 | 0.686 | 0.698 | 0.753 | 0.759 | 7469 | 3463 |

### Gold Set v4 (heading slice — informational)

| configuration | recall@5 | nDCG@5 | MRR |
|---|---|---|---|
| Baseline | 0.346 | 0.225 | 0.192 |
| BGE N=20 | 0.346 | 0.328 | 0.346 |
| BGE N=50 | 0.385 | 0.356 | 0.346 |
| BGE N=100 | 0.385 | 0.356 | 0.346 |

## Gate evaluation (ADR-0045, frozen thresholds)

| criterion | threshold | v3 result | verdict |
|---|---|---|---|
| MPI nDCG@5 Δ | ≥ +1 pp | −16.2 pp (N=20) … −18.8 pp (N=100) | **FAIL** |
| MAR MRR Δ | ≥ −0.5 pp | −14.9 pp … −14.1 pp | **FAIL** |
| MAR verified Δ | ≥ −1 pp | +10.4 … +13.8 pp (improvement) | pass |
| Abstention | hard invariant | v4 h-ab-01 baseline retrieved 2 chunks (gold set curation defect) | **FAIL (v4)** |
| Budget p95/RSS | ≤ 8000 ms / 4 GiB | N=20 v4 20.3 s over budget; v3 all within | mixed |

**Verdict: REJECT.** No combination passes on the primary slice; the v4 heading gain (+13 pp nDCG) cannot override the v3 regression.

## Failure analysis (per-query, N=20)

- **Reranker helps** entity/name-driven queries: g-sf-16 (+0.50 nDCG, "beginner's mind"), g-ent-01..07 (+0.08..+0.12, "What does the book say about X?"), g-sf-13 (+0.15).
- **Reranker destroys** concept/comparison queries: g-sf-10 (−1.00, "leverage points"), g-sf-05 (−0.85, "bounded contexts"), g-cmp-01 (−0.62, comparison). Relevant chunks are pushed out of top-5 (recall drops 0.956→0.686).
- Observed side effect: verified rate *increases* under reranking (0.655→0.79) — the gate more often labels the degraded rankings "supported"; the gate is not a substitute for ranking quality.

## What closes and what remains

- ADR-0043..0046 stand as written; this closeout records the rejection outcome (ADR-0043 closure rule: "none accepted" is a success state).
- Tickets: 01-05 resolved (built), 06 resolved (this probe + decision), 07 cancelled (activation requires acceptance), 08 resolved (this document).
- Future direction (not implemented): structure-aware ranking after vector retrieval (heading exact match / section-title similarity composed with the `Reranker` seam, ADR-0044) — benchmarked against gold set v4's heading slice, per the ADR-0047 forensic conclusions. LLM rerankers remain out of scope (ADR-0043).

## Evidence trail

- Artifacts: `/tmp/artifact_v3.json`, `/tmp/artifact_v4.json` (candidate lists, fingerprint-verified).
- Probe manifests: `/tmp/final_v3.json`, `/tmp/final_v4.json` (baseline + combinations, CI report-only, gate outcome).
- Related: `docs/research/heading-aware-embedding-forensic.md` (why heading text must not enter embeddings), `docs/adr/0047-heading-aware-embedding-representation.md` (rejected probe record).
