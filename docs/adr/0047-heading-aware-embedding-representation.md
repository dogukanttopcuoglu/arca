# ADR-0047: Heading-aware embedding representation — benchmark-rejected

Status: accepted (decision recorded)

This ADR records the evaluation of injecting heading/section context into the embedding input (representations B/C/D of the design review in `docs/research/heading-aware-embedding-design-review.md`). **The benchmark rejected all heading-aware representations. The production embedding input remains `ContentMarkdown` only; `EmbeddingVersion` is not bumped; no re-index or migration occurred.** Structural metadata (SectionPath, PageNumbers, ContentType, Citations) stays in the vector payload and belongs to filtering and post-retrieval ranking layers — never to embedding geometry.

## Problem

Section-title queries cannot reach their chunks. RCA (2026-08-06): "What does Rick Rubin mean by tuning in?" returned `no_evidence` although "Tuning In" is a real section (sec-4, page 11, chunks `rick-rubin/tuning-in/001..004`, correctly indexed in Qdrant with dense + sparse vectors and full payload). The section's chunks ranked **56 and 62** (cosine 0.51) against the query — below `RETRIEVAL_MIN_SCORE=0.6` — because the heading exists only in `VectorMetadata.SectionPath` (payload), while both the dense input (`worker.go:160`) and the sparse input (`worker.go:204, 212`) see only `ContentMarkdown`. "Tuning In" appears in no indexed text; the one chunk above 0.6 was the author biography (matched on "Rick Rubin"), which the EvidenceGate correctly rejected as unsupported.

## Candidates evaluated

- **A — ContentMarkdown only (status quo).** Baseline; kept.
- **B — SectionTitle + Content.** Rejected by benchmark: fixes only within-book verbatim-title queries while regressing content-driven retrieval.
- **C — SectionPath + Content.** Rejected by benchmark: same regression profile as B (not equivalent to B — rankings differ on 41/51 queries — but equally regressive).
- **D — BookTitle + SectionPath + Content.** Rejected by benchmark: largest embedding drift (avg cos(A,D)=0.8845 vs 0.9512 A→B), unreliable title signal (rick-rubin resolved to "Robert Henri"), and cross-book disambiguation did not materialize.

## Benchmark outcome: REJECT (evidence: `docs/research/heading-aware-embedding-forensic.md`)

GPU-reindexed probe (4 representations, 3017 chunks each, Gold Set v3 regression canary + v4 heading slice, runner-validated replica 0.7376 vs 0.738):

| rep | v3 nDCG@5 | v4 (heading) nDCG@5 |
|---|---|---|
| a | 0.738 | 0.225 |
| b | 0.647 (−12.3%) | 0.377 |
| c | 0.655 (−11.2%) | 0.377 |
| d | 0.545 (−26%) | 0.367 |

Gate rule (MPI +1 pp on heading slice; MAR ≤5% on v3): **no representation passed** — the heading gain (+15 pp, driven by 3 verbatim-title queries) is outweighed by systematic regression of concept/single_fact/comparison queries. Root-cause mechanism (forensic, §2/§8): the heading prefix boosts every chunk whose section heading shares a query term, overriding content relevance — and the corpus is structurally full of repeated headings (471/1120 sections with >1 chunk, 2210/3017 three-segment paths, up to 38× TOC duplicates, 21× Preface-family collisions).

## Decision

- **Embedding input remains content-only** — embedding represents semantic meaning; structural metadata belongs to retrieval/ranking layers.
- **No `EmbeddingVersion` bump** (`1.0.0` unchanged), no schema change, no migration. Production is byte-identical to the pre-probe state (verified: `arca_chunks` vs probe-A vectors cos = 1.000000).
- **Probe tooling** (`EmbeddingInputRepresentation`, `BuildEmbeddingInput`, `arc eval embed-probe`, gold set v4 `heading` slice) remains as an experiment surface only — it never shares a decision mechanism with production config (ADR-0042 distinction). The production default is `RepresentationContent`; the composition root never sets the option.
- **Future direction (not implemented):** if heading information is used, it must be applied after vector retrieval — a structure-aware ranking/reranking layer (heading exact match, section-title similarity, hierarchy proximity) composed with the existing `Reranker` seam (ADR-0044), behind the same benchmark gate. The v4 heading slice exists as the future benchmark for that layer.

## Rollback plan

Not applicable — nothing was activated. Rejection behavior equals the pre-probe state by construction.

## Consequences

- ADR-0047 closes as a recorded, benchmark-rejected decision; production embedding unchanged.
- Gold Set v4 (`heading` intent) committed as the benchmark slice for future structure-aware ranking work.
- Probe collections (`arca_probe_a/b/c/d`) were temporary Qdrant artifacts and have been removed; evidence lives in the forensic report.
- Embedding boundary rule (normative): `BuildEmbeddingInput` must stay `RepresentationContent` in production; any future representation change requires its own probe and version bump.
