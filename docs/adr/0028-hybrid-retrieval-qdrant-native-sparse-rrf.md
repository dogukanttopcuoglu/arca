# Hybrid retrieval: Qdrant-native sparse vectors with a SparseEncoder seam and RRF fusion

Status: proposed

Hybrid retrieval fuses Dense and Sparse streams via the existing `HybridRetriever` (RRF, k=60). The sparse stream is produced by a `SparseEncoder` seam at indexing time and persisted as Qdrant-native sparse vectors in the same collection as the dense vectors — retrieval state is never process-local.

## Decision

- **Single source of truth:** dense vectors, sparse vectors, and the chunk payload (`content_markdown` + metadata) all live in the same Qdrant collection. No second persistence layer (no Bleve/Tantivy/file-backed BM25 index) — the M2 content-persistence bug taught that retrieval state must not be process-local.
- **SparseEncoder seam:** a new indexing-stage component, symmetric to the dense embedding provider, converting a `KnowledgeChunk` into a sparse vector. M3 ships one implementation: **BM25 term weights** (lowercase normalization, alphanumeric tokenization, corpus-wide IDF). SPLADE/learned-sparse/FastEmbed encoders remain swappable behind the seam. M3 explicitly excludes stemming, synonyms, stop-word tuning, and language-specific analyzers — those are post-benchmark tuning iterations.
- **Storage lifecycle:** the Qdrant collection must be recreated with a sparse-vector config (collection config is immutable) — one full re-index, following the procedure already proven during the content-persistence backfill. IDF is corpus-global and computed at index time; refresh on corpus change is documented, not automated, in M3.
- **Fusion:** the existing `HybridRetriever` (dense stream + sparse stream, ReciprocalRankFusion, truncate to TopK) is wired into the composition root. `arc ask` keeps dense as the default mode in M3; hybrid is opt-in via retrieval-mode configuration and via `arc eval --mode hybrid`.
- **Threshold semantics (mode-aware):** `MinScore` applies per-stream before fusion (the dense stream enforces the cosine threshold; the sparse stream has its own score scale). Abstention (no_evidence) fires when no stream yields results after thresholding — no fused results, no LLM call.

## Rationale

- Reusing Qdrant for sparse avoids a second persistence mechanism, keeps cross-process correctness (arc inspect, arc ask, arc-server, and future MCP all see the same sparse index), and matches the architecture symmetry: DenseEncoder and SparseEncoder are both members of the indexing pipeline.
- RRF fusion was already implemented and tested (`internal/retrieval/hybrid`); the missing piece was the sparse stream and its wiring.
- The benchmark (ADR-0027) decides whether hybrid actually wins per category; RRF constant and BM25 details are tuning knobs measured against the committed dense baseline.

## Consequences

- Requires collection recreation + full re-index (procedure exists; one-time cost).
- BM25 IDF is single-corpus in M3; growing corpora need an IDF refresh lifecycle (documented, later milestone).
- The store seam (`VectorPoint`, upsert/search/list/delete) and the retriever seam (`seam.Retriever`) both extend to carry sparse vectors; the domain `VectorMetadata` stays untouched.
- Hybrid gains are gated by `arc eval --mode hybrid` vs the dense baseline — if hybrid does not win per category, it is not merged by default.

## Known limitation — sparse IDF staleness under incremental indexing

**Deliberate trade-off, not a bug.** BM25 IDF statistics are computed at
index time. The benchmark's Corpus Fingerprint guarantees deterministic
evaluation for a **static corpus** (index-time and query-time statistics are
identical by construction). When the corpus grows incrementally (a second
document is indexed), previously stored sparse vectors keep their old weights
while new chunks use refreshed statistics — mathematically inconsistent global
IDF. Corpus-wide sparse recomputation (re-encode + re-upsert) is an M4 concern;
contributors must not "fix" the staleness ad hoc within M3 scope.
