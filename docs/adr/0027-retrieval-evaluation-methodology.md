# Retrieval evaluation methodology: gold set, corpus fingerprint, calibration-first gates

Status: proposed

ARC measures retrieval independently from generation. A versioned, human-curated, chunk-level Gold Set built exclusively from the real indexed corpus is evaluated by `arc eval` — a benchmark runner that executes against the real composition root (real embeddings, real vector store), hard-fails on Corpus Fingerprint mismatch, and emits fully reproducible reports. Retrieval regressions and generation-quality changes become separate failure modes instead of one combined score.

## Decision

- **Gold Set:** committed, versioned JSON (`internal/eval/testdata/goldset_v*.json`) with ~50 queries across six intent categories (`single_fact`, `concept`, `procedural`, `comparison`, `entity`, `abstention`). Every query declares `expected_chunk_ids`, `expected_sections`, and `expected_no_evidence`. Built only from the real indexed corpus — no synthetic chunks, no injected facts, no LLM-generated documents (the Def Jam incident was caused by treating a synthetic artifact as production data).
- **Corpus identity:** each Gold Set declares the expected Corpus Fingerprint (SHA-256 over sorted `ContentHash` values of the gold document's indexed chunks). The runner asserts fingerprint equality against the live index before evaluating any query and aborts on mismatch — no degraded scores.
- **Retrieval/generation separation:** the primary benchmark measures retrieval only (`Recall@K`, `Precision@K`, `MRR`, `nDCG` over binary relevance, plus no-evidence precision from Abstention Queries) with no LLM involvement. Generation metrics (faithfulness, citation accuracy, completeness) form a second evaluation layer built on top of retrieval, not part of the primary benchmark.
- **Calibration-first gates:** M3 lands the harness, runs the dense baseline on the real corpus, commits the baseline report, and only then establishes regression gates (per-mode, per-primary-metric; 5% regression tolerance vs committed baseline, absolute floors at baseline − 5 points). Thresholds are never invented before measurement.
- **Reproducibility:** every report records git commit, timestamp, corpus fingerprint, document/chunk counts, embedding provider/model, retrieval mode, TopK, MinScore, RRF parameters, reranker (if any), collection name, and benchmark duration.

## Rationale

- Unmeasured retrieval changes were the source of the M2/M3 confusion (the Def Jam detour was an unverifiable hypothesis). A deterministic benchmark with corpus identity proof makes every future change (hybrid, RRF tuning, reranking, query decomposition, KG traversal) an experiment against a fixed baseline.
- Separating retrieval from generation keeps regressions diagnosable: a retrieval regression, a reranker regression, and an answer-synthesis regression fail independently.
- The fingerprint assert turns "wrong corpus" into a hard benchmark failure instead of silently misleading metrics.

## Consequences

- Retrieval changes must not merge on intuition: each lands with an `arc eval` delta vs the committed baseline for the same mode.
- The benchmark corpus PDF is copyrighted and not in the repository; the full benchmark runs locally or via the on-demand CI job (never in the always-on path).
- Binary relevance caps nDCG sensitivity; graded relevance is a possible later refinement if binary nDCG proves limiting.
- Query decomposition, query rewriting, multi-step retrieval, reranking, and KG traversal are explicitly deferred (M3.1/M4) and gated by this benchmark.
