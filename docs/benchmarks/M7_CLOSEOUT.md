# M7 GraphRAG — Production Readiness Closeout

**Status:** PRODUCTION-READY (closed 2026-08-05)
**Scope:** entity-only knowledge graph retrieval, benchmark-gated (ADR-0038…0042)
**Verification:** production engineering audit (8 domains) + live validation + fix verification

## What shipped

| # | Commit | Deliverable |
|---|---|---|
| 01 | `db70390` | GraphStore seam + QdrantGraphStore (vectorless REST, atomic payload schema) |
| 02 | `c6f410c` | Worker entity graph writes tied to the diff lifecycle |
| 03 | `ecbda8c` | GraphRetriever deterministic entity-overlap production rebuild |
| 04 | `1c0dc1a` | GraphFusionRetriever + GraphFusionConfig (RRFK frozen) |
| 05 | `6cb8405` | Eval surface (`--graph-weight` / `--graph-only`) |
| 06 | `462fee9` | Gold Set v3 sweep — **GraphWeight 1.0 frozen** |
| 07 | `e7305ac` | Orchestration gate active (entity intent + UseGraph) |
| B1 | `e7a17ac` | Content resolution from the vector store payload (review finding) |
| B2 | `e225bb5` | Repeated gate evaluation (`--gate-runs`, median) for LLM variance |
| A1/A2 | `bf5278d` | Race-free AddNode via atomic field appends (audit finding) |

## Audit closeout

| Finding | Severity | Status |
|---|---|---|
| A1 — AddNode lost update under concurrency (measured 19/20) | High | **FIXED** (`bf5278d`): atomic `set_payload` field appends; live re-test 0/10 |
| A2 — GetNode-failure overwrite | Medium | **FIXED** (same commit): `ensurePoint` creates only on HTTP 404; transient errors abort |
| B1 — ensureCollection per-write GET | Low | **BACKLOG** (performance; no correctness impact) |
| BULGU-1 — graph content empty in production | Medium | **FIXED** (`e7a17ac`): vector store payload resolution; live 0 → 10.6k/8.7k chars |
| BULGU-2 — gate metric variance | Low | **MITIGATED** (`e225bb5`): `--gate-runs N` median; variance band documented (ADR-0040) |

## Acceptance criteria (Gold Set v3, frozen w1.0)

- Entity slice: recall 0.276 → **0.840** (≥ dense × 1.05 ✓), MRR 0.458 → **0.938** ✓
- Entity-outside: recall 1.000 flat, MRR −2.7% (≤5% tolerance ✓)
- Abstention: graph leak 0; M5 gate metrics non-regressing ✓
- Recall-regression counter: 0; retrieval metrics byte-identical on re-run ✓

## Unchanged contracts

M4 `FusionPolicy`, M5 `EvidenceGate` (MaxTokens provider-capability adjustment only), M6 `RetrievalDecision`/`IntentHint` evolution via the ADR-0037 evidence rule, Gold Set v2, retrieval scoring, fusion behavior, orchestration routing. Benchmark validity preserved: calibration and fixes never altered retrieval semantics.

## Known boundaries (documented, accepted)

- Same-document concurrent writers can still race the per-document score field (supported model: one writer per document; ADR-0038 errata).
- Entity intent detection covers the benchmark-proven question forms; other entity phrasings are unmeasured.
- B1 (ensureCollection GET) remains in the performance backlog.

## Next milestone candidates

Deep research (AgentEngine/QAJob), rerankers, multi-KnowledgeSpace isolation — all measured against the frozen M7 artifacts (`M7_CALIBRATION.md`, Gold Set v3, `m7_fusion_w10.json`).
