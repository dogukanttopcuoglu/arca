# Structure-Aware Reranking — Research Document

> Status: research only — no implementation, no production change
> Date: 2026-08-06
> Predecessors: M8 closeout (`docs/benchmarks/M8_CLOSEOUT.md`), ADR-0043..0047, `docs/research/heading-aware-embedding-forensic.md`
> Question: can structure-aware reranking recover the v4 heading gains (BGE +13pp nDCG) without the v3 regression that rejected global reranking?

---

## 1. Problem statement

M8 proved that a **global cross-encoder reranker** degrades ARC's production retrieval: on Gold Set v3 (37 queries, production distribution), BGE-reranker-v2-m3 at any candidate budget N lowered recall@5 (0.956→0.686), nDCG@5 (0.886→0.698) and MRR (0.902→0.753) — a hard reject on the frozen MPI/MAR thresholds. Yet the same reranker **improved** the v4 heading slice (+13pp nDCG, +15pp MRR). The reranker is not uniformly bad: it wins on entity/name queries and loses heavily on concept/comparison queries.

Two questions follow:

1. Can a **selective** reranking policy (activated only where the evidence shows gains) capture the entity + heading gains?
2. Can **deterministic structural signals** (section path, page proximity, hierarchy) achieve the heading gains *without a model at all* — the direction the ADR-0047 forensic report pointed to?

The constraint is production stability: retrieval correctness first, reranking second. "Do not implement" is an acceptable conclusion.

## 2. M8 findings (evidence)

| slice | baseline | BGE best | Δ | verdict |
|---|---|---|---|---|
| v3 nDCG@5 | 0.886 | 0.698 (N=100) | **−18.8pp** | MPI fail |
| v3 MRR | 0.902 | 0.753 | **−14.9pp** | MAR fail |
| v3 recall@5 | 0.956 | 0.686 | −27pp | recall destroyed |
| v4 nDCG@5 | 0.225 | 0.356 (N=50/100) | **+13.1pp** | gain |
| v4 MRR | 0.192 | 0.346 | **+15.4pp** | gain |
| v4 abstention | h-ab-01 baseline retrieved 2 chunks | — | — | gold-set curation defect, invariant broken |

Per-query failure pattern (M8 closeout §Failure analysis):

- **Wins:** entity/name queries — g-sf-16 "beginner's mind" (+0.50), g-ent-01..07 "What does the book say about X?" (+0.08..+0.12), g-sf-13 (+0.15).
- **Losses:** concept/comparison — g-sf-10 "leverage points" (−1.00), g-sf-05 "bounded contexts" (−0.85), g-cmp-01 (−0.62).
- Side effect: verified rate rose (0.655→0.79) while ranking quality fell — the gate labels degraded rankings "supported"; verified rate is not a ranking-quality signal.

## 3. Why global reranking failed

1. **Cross-encoder score ≠ retrieval relevance for this corpus.** BGE re-ranks by (query, chunk) pairwise similarity; for concept queries the top-5 displacement pattern (forensic §2: g-sf-07 "repository" section jump 5th→1st; g-sf-14 section displacement) shows the model overweights lexical term hits in *content* while ignoring the structural context that GraphFusion's RRF already encodes.
2. **Recall was destroyed, not just ranking.** Relevant chunks were pushed out of top-5 (recall −27pp). A reranker that cannot see beyond the candidate set cannot fix what it displaces.
3. **Uniform application.** The same reranker ran on all intents. Entity gains and concept losses averaged to a net loss — the aggregate hid a bimodal distribution.
4. **No structural context.** The reranker received `ContentMarkdown` only (plus metadata in `SearchResult` that it never used). Section path, page numbers, and hierarchy were available in the payload and unused.

## 4. Available structural signals

### 4.1 Available today in the retrieval path (payload-verified)

The `SearchResult.Metadata` carries `VectorMetadata` (payload keys verified on a live point): `document_id`, `chunk_id`, `chunk_order`, `section_path`, `page_numbers`, `content_type`, `citations`, plus `content_markdown`.

| signal | source field | notes |
|---|---|---|
| Section path | `section_path` ("Tuning In"; "Thinking in Systems > A Brief Visit to the Systems Zoo > One-Stock Systems") | full hierarchy, segmentable; depth derivable (73% of chunks have 3-segment paths) |
| Section depth | `section_path` segment count | cheap, deterministic |
| Page numbers | `page_numbers` | citation + proximity signal |
| Content type | `content_type` (paragraph/table/code/list/equation/figure) | typed filtering |
| Chunk order | `chunk_order` | linear reading position |
| Citations | `citations` | bibliography/inline reference text |
| Document identity | `document_id` | cross-book disambiguation (5-book corpus) |
| Heading exact match | query tokens ∩ `section_path` tokens | **the key heading signal — lexical, model-free** |

### 4.2 Available in inspection output but NOT in the payload (required data if used)

| signal | where it lives | payload today |
|---|---|---|
| Parent chunk | `KnowledgeChunk.ParentChunkID` | **absent** |
| Child chunks | `KnowledgeChunk.ChildChunkIDs` | **absent** |
| Prev/next neighbor | `KnowledgeChunk.PreviousChunkID/NextChunkID` | **absent** (chunk_order exists, IDs don't) |
| Semantic node level | `SemanticNode.Level` | absent (derivable from section_path depth, lossy) |
| Entity graph locality | `arca_graph_nodes` payload (`chunk_<hex>` evidence) | separate collection, queryable |

Any experiment using parent/child/neighbor continuity requires a small payload addition (three ID fields) — a schema change that itself needs an ADR; until then those signals are unavailable to a post-retrieval reranker.

## 5. Candidate approaches

### 5.1 Pure cross-encoder reranking (global) — M8, rejected

- **Evidence:** v3 −16 to −19pp nDCG, recall destroyed.
- **Status:** baseline for comparison; not a candidate.

### 5.2 Structure-aware reranking (deterministic, model-free)

Score = GraphFusion score + small structural bonus, applied **selectively**:

- **Heading overlap:** fraction of query tokens present in `section_path` tokens (lexical; the v4 gain mechanism without an embedding change — forensic showed heading *ranking* signals are valuable exactly where heading *embedding* failed).
- **Page proximity:** candidate page numbers near query-relevant pages (when known) or near other high-scoring candidates.
- **Neighbor continuity:** chunk_order adjacency of candidates (two candidates at orders 5 and 6 are likely one coherent passage).

- **Hypothesis:** the v4 heading gains are largely lexical-structural (section title in query + path in payload); a deterministic bonus recovers them with zero model cost and full determinism.
- **Risk (forensic §8):** heading overlap alone can promote wrong sections (g-sf-07: "repository" in an unrelated section heading). Mitigation: the bonus is small, additive, and gated to heading-style queries — never a primary signal.

### 5.3 Query-conditioned / intent-gated reranking (Option A)

Route by intent, applying different rerankers per class:

- entity intent → cross-encoder rerank (M8 showed entity wins)
- heading/navigation intent → structure bonus (5.2)
- concept/comparison/other → GraphFusion unchanged (no rerank)

- **Hypothesis:** the M8 bimodal outcome is *explained by intent*; gating recovers both gains and avoids both losses.
- **Required:** reliable intent classification for the heading class (the analyzer has entity pattern; a heading/navigation classifier does not exist — new deterministic rules: "What does the book say about X?" is already entity; navigation = query tokens match section-path tokens of retrieved candidates? circular — needs care).
- **Risk:** intent classification errors route queries to the wrong policy. M6's discipline: intent signals are hints, the orchestrator holds policy, benchmark gates every new intent class.

### 5.4 Hybrid scoring (Option B) — additive weighted score

FinalScore = α·GraphFusionScore + β·SemanticScore + γ·StructureScore.

- **Rejected as primary design:** RRF already fuses rank-based signals; re-weighting raw scores post-hoc reintroduces the scale-comparability problem RRF exists to avoid (M4 rationale). Any linear combination needs calibration on data we do not have enough of (51 gold queries) and would silently change GraphFusion's frozen behavior for every query — a violation of the "GraphFusion is the primary signal, reranking is selective/additive" constraint.
- **Verdict:** keep as a degenerate case of 5.2 with α=1, β=0, γ small, gated — not a general mechanism.

### 5.5 Learning-to-rank

- **Rejected:** the gold set is 51 queries across 5 books — insufficient training data for any LTR model; it would require a new labeled corpus (ADR-0027 discipline: gold set is the test set, never the training set). Not viable; noted for completeness only.

### 5.6 Structure filter then optional rerank (Option C)

Filter candidates by structure (e.g. drop candidates whose section has zero query-token overlap on heading queries) then rerank the survivors.

- **Risk:** filtering removes recall (the exact failure mode of M8 — a filter is a hard decision). A **soft bonus** (5.2) is strictly safer than a hard filter; hard filters are rejected.

## 6. Proposed candidate architecture

```
GraphFusionRetrieve (unchanged, primary signal)
        │
        ▼
Intent classification (deterministic, rule-based, hints only)
        │
        ├── entity ────────────────► cross-encoder rerank (M8 entity wins)
        │                                │
        ├── heading/navigation ────► deterministic structure bonus
        │                                │   heading overlap (query ∩ section_path)
        │                                │   page proximity
        │                                │   neighbor continuity (if payload extended)
        │                                ▼
        └── concept/comparison/other ──► GraphFusion result unchanged
                                           (no rerank — M8 losses avoided)
```

Principles:

1. **Selective, not global** — only entity and heading intents touch the ranking; concept/comparison keep GraphFusion byte-identical (M8 loss classes untouched).
2. **Additive, not replacing** — the structure bonus is a small delta on the existing RRF ordering, never a new primary score (5.4 rejected).
3. **Model-free heading path** — the heading gain mechanism is lexical-structural; a deterministic bonus is cheaper, deterministic, and auditable vs another model.
4. **Abstention preserved** — empty retrieval and the EvidenceGate keep their current behavior on every path; no new abstention semantics.
5. **Entity gate reused** — the M7 `UseGraph` gate already isolates entity queries; the entity rerank rides the same gate (graph path already swaps the execution component per intent — ADR-0042 precedent).

Open design questions (not resolved here):

- Heading-intent detection: no classifier exists. Candidate deterministic rule: query contains a phrase that matches a retrieved candidate's section_path tokens (post-retrieval check — but that makes the policy retrieval-dependent; M6's orchestrator is retrieval-independent). Alternative: treat "What does the book say about X" as entity (existing) and add "section-title-like" detection later with its own benchmark.
- Where the bonus applies: pre-truncation (on N candidates) vs post-truncation (on top-5). Pre-truncation is required for any rerank to have leverage; the bonus must apply before the final truncation.

## 7. Experiment plan

Common setup: existing artifacts (M8 `/tmp/artifact_v3.json`, `/tmp/artifact_v4.json`), gold sets v3 + v4, GPU environment, fingerprint-gated runner, frozen thresholds (MPI +1pp nDCG@5, MAR ≤5% per slice, abstention hard invariant, ADR-0027 5% regression tolerance).

### E1 — Intent-gated cross-encoder rerank

- **Hypothesis:** reranking only entity + heading queries captures the M8 gains (entity +0.08..+0.5, heading +13pp) while concept/comparison slices stay untouched at baseline.
- **Required data:** M8 artifacts (exist); intent labels per gold query (gold set `intent` field — v3 already labels entity/comparison/concept; heading queries are v4's `heading` intent).
- **Offline benchmark:** rerank only entity (v3) + heading (v4) queries; leave concept/comparison/single_fact/procedural on GraphFusion. Measure per-slice nDCG@5/MRR/recall@5.
- **Success:** entity slice nDCG Δ ≥ +1pp AND v4 heading nDCG Δ ≥ +1pp AND all other v3 slices within 5% of baseline AND abstention unchanged.
- **Failure:** any non-reranked slice regresses >5%; entity gain disappears at budget ≤100; v4 gain < MPI.

### E2 — Deterministic structure bonus on heading queries (model-free)

- **Hypothesis:** a small lexical heading-overlap bonus (query tokens ∩ section_path tokens, normalized) plus page-proximity ordering recovers the v4 heading gain with zero model, full determinism, no GPU.
- **Required data:** payload section_path/page_numbers (exist); no schema change for E2's core (parent/child signals excluded).
- **Offline benchmark:** v4 heading slice with bonus vs baseline; v3 untouched (bonus gated to heading intent).
- **Success:** v4 nDCG Δ ≥ +1pp, MRR Δ ≥ 0, determinism (fixed-seed rerun identical), v3 slices within 5%.
- **Failure:** v4 gain < MPI; bonus distorts any v3 slice (should be impossible by gating — if it leaks, the gate failed).

### E3 — Combined: entity cross-encoder + heading structure bonus

- **Hypothesis:** the two mechanisms are complementary and non-interfering (disjoint intent classes).
- **Required data:** E1 + E2 outputs.
- **Offline benchmark:** full v3+v4 with both policies active.
- **Success:** E1 and E2 criteria jointly; combined v3 aggregate within 5% of baseline (net-zero expected: entity gains offset by nothing, concept untouched).
- **Failure:** interaction effects regress any slice; combined aggregate regresses >5% vs baseline.

### E4 — Control: global rerank (reference)

- **Hypothesis:** replicates the M8 rejection (sanity check of the harness).
- **Required data:** M8 artifacts.
- **Success:** reproduces M8 REJECT (Δ ≈ −16 to −19pp on v3).
- **Failure:** harness drift (results differ materially from M8 closeout).

### E5 (optional, deferred) — Neighbor/parent-child continuity

- **Hypothesis:** chunk_order adjacency and parent-child coherence add ranking precision on narrative passages.
- **Required data:** payload extension (ParentChunkID/ChildChunkIDs/Previous/NextChunkID) — a schema change needing its own ADR and full re-index; explicitly deferred until E2 proves the deterministic path is insufficient.

## 8. Recommendation

**Proceed to E2 first (deterministic structure bonus, heading-gated), then E1 (intent-gated entity rerank), then E3.**

Rationale:

1. E2 is the cheapest experiment with the largest evidence base: the v4 heading gain mechanism is lexical-structural (section title in query, path in payload), the forensic report already showed heading *ranking* signals are valuable where heading *embedding* failed, and the bonus is deterministic — no model, no GPU, no schema change, trivially auditable.
2. E1 isolates the only M8 win class (entity) behind an existing gate (M7 `UseGraph`), keeping the M8 loss classes (concept/comparison) byte-identical.
3. E3 is the composition test; if E1 or E2 individually fails, E3 is not attempted.
4. The "do not implement" bar is low and explicit: if E2's v4 gain does not clear MPI (+1pp), the heading path closes and only E1 remains; if E1's entity gain also fails, structure-aware reranking is closed with a research record, matching the M8 closeout pattern.

## 8a. E2 executed — results (2026-08-06, GPU, fingerprint-gated)

**Hypothesis tested:** a deterministic heading-overlap bonus (query tokens ∩ most-specific heading, score = content × (1 + 0.5·overlap)) recovers the v4 heading gain with zero model.

| configuration | v4 nDCG@5 | v4 MRR | v3 (gated) | v3 (ungated) |
|---|---|---|---|---|
| baseline | 0.225 | 0.192 | 0.886 | 0.886 |
| structure N=20 | **0.267 (+3.85pp)** | 0.246 | 0.886 (+0.00) | 0.879 (−0.66pp) |
| structure N=50 | 0.237 (+1.15pp) | 0.231 | 0.886 (+0.00) | 0.867 (−1.78pp) |
| structure N=100 | 0.237 (+1.15pp) | 0.231 | 0.886 (+0.00) | 0.867 (−1.78pp) |
| BGE N=50 (reference) | 0.356 (+13.1pp) | 0.346 | −16.2pp (rejected) | — |

**Findings:**

1. **The heading gain is mostly model-driven, not lexical.** The deterministic bonus captures only ~30% of BGE's v4 gain (+3.85pp vs +13.1pp at best N). The cross-encoder's semantic (query, section-content) matching is doing most of the work; heading-token overlap alone is a weak signal on this corpus.
2. **The gate is selective-safe.** Intent-gating works exactly as designed: v3 heading-gated results are bit-identical to baseline (delta +0.00 on recall/nDCG/MRR — verified). Ungated, the bonus costs only −0.66..−1.78pp nDCG (vs BGE's −16..−19pp), with MRR at the MAR boundary (−1.7pp at N≥50).
3. **v4 BGE passes every acceptance threshold once the abstention defect is fixed** (nDCG +13.1pp, MRR +15.4pp, verified +15.4pp, abstention 0 candidates, p95 4.7s ≤ 8s, RSS 3.5GB ≤ 4GiB) — but the M8 decision stands: acceptance is defined on the production distribution (v3), where BGE fails MPI/MAR. The v4-only acceptance is a slice artifact, not a production green light.
4. Gold set v4 abstention query was defective (name collision: "Rick Rubin" matched the author bio at 0.635); replaced with the proven abstention from v3 ("capital of Atlantis", 0 candidates) — committed.

**E2 verdict:** the deterministic structure path clears MPI on the heading slice but is far weaker than the model path and does not change the M8 rejection. E1 (entity-gated BGE) remains the only untested candidate; E3 is meaningful only if E1 passes.

## 9. Explicit non-goals

- **No global reranking** over all retrieved chunks (M8 rejected; E4 is a control, not a candidate).
- **No heading/section text in embedding input** (ADR-0047 rejected; forensic evidence stands).
- **No embedding geometry or schema change** for the core experiments (E5's payload extension is deferred and requires its own ADR).
- **No learning-to-rank** (gold set is the test set, 51 queries; insufficient training data by design — ADR-0027).
- **No hybrid weighted scoring as a general mechanism** (5.4 — RRF exists to avoid score-scale fusion; the bonus in E2 is gated, small, and additive, not a new primary score).
- **No hard structural filters** (5.6 — filters destroy recall; soft bonuses only).
- **No new abstention semantics** — empty retrieval and EvidenceGate behavior unchanged on every path.
- **No reranker activation without the full gate** (MPI/MAR/abstention/5% tolerance, frozen before measurement).

## 10. Conclusion

Structure-aware reranking is a **promising but unproven** direction, and the research record is explicit that it may conclude with "do not implement". The evidence ordering is clear: the heading gain is structural-lexical (test it deterministically first, E2), the entity gain is model-driven (test it gated, E1), and only their composition (E3) can justify any production change. GraphFusion remains the primary retrieval signal; anything that touches it must win its own benchmark first.
