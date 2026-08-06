# M7 GraphRetriever strategy: deterministic entity-overlap scoring, benchmark-gated

Status: accepted

M7's `GraphRetriever` is rebuilt from the hardcoded scaffold into a real retrieval signal: deterministic lexical entity-overlap scoring over the entity-only graph (ADR-0038). Its job is not to answer queries — it deterministically produces chunk candidates carrying entity evidence. The kill-gate prototype measured the ceiling of exactly this behavior (+126%/+174% dense-baseline recall on entity queries).

## Decisions

- **No query-side extraction.** The retriever applies deterministic lexical normalization to the query (lowercase, punctuation trim, stopword removal) and matches against normalized entity names via substring containment. LLM/rule-based NER query extraction, entity-embedding similarity, and alias resolution are out of scope; the lexical approach is what the benchmark measured and keeps the signal debuggable and deterministic.
- **Match rule: `matched_tokens > 0`.** A single normalized query token contained in an entity name is a match — the prototype behavior. False positives (e.g. "bank" → "World Bank") are accepted and suppressed in the scoring layer, not by tightening the match rule (stricter rules cost multi-token entity recall).
- **Score: `Σ(entity.Score × token_coverage)`** with `token_coverage = matched_tokens / entity_token_count`. Full matches get coverage 1.0 and reproduce the prototype scores; partial matches are proportionally damped. Chunks carrying multiple matched entities sum their contributions (stronger evidence). Ranking: score desc, tie-break chunk ID asc — a total order, so Go map iteration randomness is neutralized.
- **Output contract:** `NewGraphRetriever(store GraphStore, contentStore ContentStore)`. `SearchResult` carries `ChunkID`, `Score`, `ContentMarkdown`, and minimal `Metadata{ChunkID}`. Section/document metadata is not populated in v1 (accepted trade-off: citations may show unknown sections); the ADR-0038 node schema stays unchanged. Results are truncated to `query.TopK`.
- **Content resolution (errata 2026-08-05):** chunk markdown is resolved from the vector store point payload first — the same source of truth as the dense retriever — because production indexing and querying run in different processes and the process-local `ContentStore` is empty. The `ContentStore` remains the fallback for payload gaps (legacy/test composition), wired via the `WithVectorStore` option; the `NewGraphRetriever(store, contentStore)` signature is unchanged. Content resolution happens after ranking and never alters result ordering.
- **MinScore applied.** The graph stream applies `query.MinScore` like the dense stream — a consistent retrieval contract across streams; low-coverage matches below the threshold do not enter the stream. The prototype's absolute numbers are not the evidence; the relative ceiling is re-verified in the ticket-07 benchmark under the same config (TopK 5, min-score 0.6).
- **Determinism guarantee.** Same graph state + same query → byte-identical result list. Regression tests: (a) two consecutive calls produce identical lists, (b) equal-score results keep stable chunk-ID ordering.
- **Scaffold removal.** The `*InMemoryGraphStore` type-assertion and the `chk-1`/"creativity" fixtures leave the production path; the retriever works only through the `GraphStore`/`ContentStore` seams.

## Rationale

- Every decision preserves the measured ceiling: the scoring formula degrades exactly to the prototype on full matches, the match rule is unchanged, and MinScore only removes below-threshold candidates (verified in the benchmark rather than re-derived here).
- The retriever stays a pure execution component: no policy (fusion weighting, orchestration gates — tickets 06/08), no answer logic, no new extraction.

## Consequences

- `internal/graph/retriever/retriever.go` is rewritten on the seams; determinism and scoring tests are added at the retriever seam (unit: fake graph + content store).
- Fusion integration (ticket 06), benchmark dimension (ticket 07), and the orchestration gate (ticket 08) remain open; the graph stream enters RRF only after the ticket-07 acceptance rule is met (entity-category ≥5% gain over dense, no >5% regression elsewhere, gate metrics preserved).
