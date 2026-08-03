# M3 Retrieval Decision — Dense vs Sparse vs Hybrid

**Date:** 2026-08-03
**Decision authority:** benchmark evidence (ADR-0027), not intuition
**Data:** `baseline_dense_v1.json`, `baseline_sparse_v1.json`, `baseline_hybrid_v1.json` — all run on Gold Set v1, corpus fingerprint `51b1909e…` (196 chunks), TopK 5, MinScore 0. Dense metrics re-ran **byte-identical** (recall 0.740 / MRR 0.666 / nDCG 0.645) across two independent runs — reproducibility confirmed.

## 1. Per-intent analysis (mean per query)

| intent | q | mode | recall@5 | precision@5 | MRR | nDCG@5 |
|---|---|---|---|---|---|---|
| single_fact | 9 | dense | 0.963 | 0.378 | 0.815 | 0.849 |
| | | sparse | 0.870 | 0.333 | 0.843 | 0.817 |
| | | **hybrid** | **0.963** | **0.378** | **0.944** | **0.935** |
| entity | 8 | dense | 0.750 | 0.200 | 0.625 | 0.658 |
| | | sparse | 0.875 | 0.250 | 0.729 | 0.774 |
| | | **hybrid** | **0.875** | **0.250** | **0.750** | **0.781** |
| concept | 10 | dense | 0.700 | 0.280 | 0.658 | 0.614 |
| | | sparse | 0.700 | 0.260 | 0.617 | 0.584 |
| | | hybrid | 0.683 | 0.260 | 0.658 | 0.580 |
| procedural | 8 | dense | 0.625 | 0.325 | 0.729 | 0.605 |
| | | sparse | 0.354 | 0.150 | 0.244 | 0.240 |
| | | hybrid | 0.615 | 0.300 | 0.656 | 0.537 |
| comparison | 8 | dense | 0.646 | 0.250 | 0.483 | 0.481 |
| | | sparse | 0.583 | 0.225 | 0.452 | 0.440 |
| | | hybrid | 0.521 | 0.200 | 0.667 | 0.495 |
| abstention | 8 | dense | — | — | — | — |
| | | sparse | — | — | — | — |
| | | hybrid | — | — | — | — |

Abstention: all modes return top-k neighbors at MinScore 0 (no_evidence_precision 0.000) — the threshold question is orthogonal to mode selection (T5, calibrated at 0.6).

**Where hybrid helps:** single_fact (MRR +0.129, nDCG +0.086) and entity (recall +0.125, nDCG +0.123).
**Where hybrid hurts:** procedural (MRR −0.073, nDCG −0.068) and comparison recall (−0.125).

## 2. Error analysis

### Regressions (hybrid recall < dense): 5 queries

Mechanism: **BM25 exact-term crowding.** The sparse stream ranks chunks sharing generic tokens ("play", "self", "make", "rule", "gate") above the semantically relevant dense hits, displacing them past the top-5 fold.

- **rr-016 (concept, play):** dense finds play/001-003; hybrid's BM25 injects the-ecstatic/002, expect-a-surprise/001, look-for-clues/001 → play/001-002 pushed out. Expected `[play/001, play/002, play/003]`, hybrid retrieved only play/003.
- **rr-021 (procedural, self-doubt):** hybrid drops make-it-up/003 (expected) for completion/004 (BM25 "make"/"work" overlap).
- **rr-023 (procedural, momentum):** dense found momentum/001-002; hybrid's crowding keeps only momentum/001 in the fold.
- **rr-033 (comparison, temporary rules vs rules):** hybrid loses temporary-rules/001 (the "temporary" side) to how-to-choose/002, patience/003.
- **rr-035 (comparison, gatekeeper vs prism):** hybrid loses the-gatekeeper/001 from the fold.

MRR regressions (6 queries: rr-016/017/020/022/027/036) are the same mechanism at rank level — the relevant chunk is present but displaced below rank 1.

### Improvements (hybrid recall > dense): 3 queries

Mechanism: **exact-token rescue.** BM25 surfaces chunks dense missed entirely.

- **rr-038 (entity, Jackson Pollock):** dense retrieved none of the spontaneity chunks; BM25's exact "Pollock" match promoted spontaneity/001-002 into the fold → recall 0 → 1.0. This is the classic dense weakness the sparse stream was built for.
- **rr-024 (procedural, blocked musician):** BM25 exact-phrase match promoted write-for-someone-else/001.
- **rr-019 (concept, artificial intelligence):** BM25 promoted beginner-s-mind/002 (AlphaGo passage) that dense missed.

MRR/NDCG wins concentrate in the same profile: 11 MRR wins / 6 losses, 13 nDCG wins / 12 losses.

## 3. Statistical summary

| metric | dense | hybrid | Δ abs | Δ rel | wins | ties | losses |
|---|---|---|---|---|---|---|---|
| recall@5 | 0.740 | 0.734 | −0.006 | −0.8% | 3 | 43 | 5 |
| precision@5 | 0.288 | 0.279 | −0.009 | −3.1% | — | — | — |
| MRR | 0.666 | 0.736 | +0.070 | +10.5% | 11 | 34 | 6 |
| nDCG@5 | 0.645 | 0.668 | +0.023 | +3.6% | 13 | 26 | 12 |

**Confidence discussion (no formal significance testing):**
- Sample: 51 queries (43 relevance + 8 abstention). Per-intent cells are 8–10 queries — individual cell deltas are noise-scale; only the aggregate and the single_fact/entity clusters are meaningful.
- The MRR gain (+10.5%) is directionally consistent (11 wins vs 6 losses) but driven by two intents; procedural MRR moves the other way. The aggregate hides a negative cluster — not a uniform improvement.
- Recall/precision deltas (−0.8%/−3.1%) are within the 5% tolerance but not noise: the same 5 queries regress deterministically (BM25 crowding is a systematic mechanism, not random).
- The dense baseline is byte-reproducible across runs, so observed deltas exceed run-to-run variance; they are real effects, but their size at this sample does not support a blanket promotion.

## 4. Operational cost

Measured on the live stack (Ollama embeddings, Qdrant, per-query stats in the same process environment):

| cost | dense | sparse | hybrid |
|---|---|---|---|
| mean query latency | 56 ms | 9 ms | 74 ms |
| hybrid overhead vs dense | — | — | +18 ms (+32%) |
| storage: sparse vectors (196 chunks, ~159–326 dims each) | — | ≈ +160 KB values + index | same as sparse |
| storage: dense (768-dim × 196) | ≈ 600 KB | — | unchanged |
| indexing: BM25 encode per chunk | — | ≈ 0.1 ms/chunk | same as sparse |
| indexing: corpus stats pass | — | one ListPoints scroll + token pass | same as sparse |

Latency overhead is acceptable for the interactive CLI but real (~1/3 more per query). Storage roughly doubles vector footprint yet stays low-MB. Indexing cost is negligible vs dense embedding.

## 5. Recommendation

**Keep Dense as the default retrieval mode. Hybrid stays opt-in via `RETRIEVAL_MODE=hybrid`.**

Decision rule applied:
- recall regression within tolerance (≤5%): **pass** (−0.8%)
- ranking gains consistent across intent categories: **fail** — single_fact/entity win, procedural loses (MRR −0.073, nDCG −0.068), comparison recall regresses −0.125
- operational overhead acceptable: **pass** (+18 ms/query, negligible indexing cost)
- no major failure mode introduced: **fail** — BM25 exact-term crowding is a systematic top-5 displacement mechanism (5 recall + 6 MRR regressions, deterministic)

The evidence is mixed by the stated rule, so Dense remains the default and Hybrid stays opt-in until M4. The report's positive findings are preserved for M4 targeting: hybrid's exact-token rescue (entity +18.8% recall cluster, single_fact ranking +0.129 MRR) is real, and the crowding failure mode suggests the highest-value M4 experiment is **category- or threshold-aware fusion** (e.g., keeping the sparse stream's contribution bounded) rather than a blanket default flip.

**This document is the project's retrieval baseline for all future work** (rerankers, query decomposition, knowledge graph retrieval): any change is measured against these three reports on the same gold set and fingerprint.
