# M7 graph persistence: entity-only node store, benchmark-gated

Status: accepted

M7 stores the existing enrichment entity output as a persistent graph signal for retrieval. The decision set below is the first M7 ADR (wayfinder ticket 04); every choice is backed by measured evidence — the kill-gate prototype (+126%/+174% dense-baseline recall on entity queries) and the enrichment data-quality research.

## Decisions

- **Entity-only nodes.** Only entities are persisted as graph nodes. Concepts (100% coverage but section-path-driven attachment, H1 tier OCR-corrupt) and concept-based relations carry no measured retrieval signal and are deferred to a future iteration gated by their own benchmark.
- **No relation persistence in v1.** The remaining entity→entity relation pool under an entity-only scope is 7/7 spurious `located_in` relations (publisher city lists) — noise, not signal. The `GraphStore` seam keeps its edge API for future use, but the v1 implementation does not support it.
- **Entity score threshold ≥ 0.90.** Two independent evidence lines agree: the data-quality research (0.80 tier is 8/12 frontmatter/acknowledgment/bibliography noise) and the kill-gate prototype (the ≥0.90 slice yields the highest measured ceiling). Caveat, recorded deliberately: the score is a mention-count proxy (`0.8 + 0.1·(mentions−1)`, capped at 1.0 — one mention scores 0.80, two score 0.90), a frequency gate, not a quality estimate; it is re-calibrated when a real NER replaces the rule-based extractor.
- **Node identity: `type:lower(name)`.** A deterministic key; same-named entities merge across documents into one node (the kill gate measured its ceiling on exactly this normalized behavior). Payload: canonical name, entity type, score, and `chunk_ids` evidence references. Scope out: alias resolution, entity disambiguation, advanced entity linking. Caveat: same-name-different-entity collisions are accepted in v1 and documented as a known limitation.
- **Worker-level indexing integration.** A `WithGraphStore(graphstore.GraphStore)` worker option (mirroring `WithSparseEncoderProvider`). `ExecuteSync` writes graph state tied to the existing diff plan: upserted chunks → idempotent node upsert (chunk IDs union), deleted chunks → chunk IDs removed from nodes, nodes with no remaining evidence are deleted. `QdrantGraphStore` is node-only (one collection).
- **Persistence target: Qdrant.** A dedicated node collection behind the `GraphStore` seam (research: payload-embedded edges go stale under diff-skip re-indexing; a separate collection keeps chunk payload schema and corpus fingerprint `8b21a664…` untouched — fingerprints hash chunk ContentHashes only).

## Persistence schema errata (2026-08-05, M7 audit A1)

The original node payload stored `chunk_ids` as a list and `score` as a single value, written via read-modify-write (`GetNode` → union → upsert). Under concurrent writers — parallel indexing processes touching the same cross-document node — this lost updates (measured 19/20) and could regress the score. The Qdrant persistence schema now uses atomic field appends:

- **Chunk evidence** is stored as one payload field per chunk, `chunk_<hex(chunkID)>: true`, appended atomically via `POST /points/payload` (`set_payload`). Concurrent writers add disjoint fields; the union is structural, not client-computed.
- **Score** is stored per document as `score_<hex(documentID)>: <score>`; reads take the max across documents, so the score never regresses under concurrent writes.
- **Removal** uses atomic field clears (`clear_payload`) per document in `DeleteByDocument`; nodes left without evidence fields are deleted.
- **Hex encoding** avoids Qdrant's JSON-path interpretation of `/` and `.` in raw chunk/document IDs (verified on Qdrant 1.18).
- **Point creation** (`ensurePoint`) creates the point with only static fields and only on HTTP 404; transient errors abort (M7 audit A2 closure) instead of replacing an existing point's evidence.
- **Same-document concurrency boundary:** the per-document score field is last-write-wins within one document; concurrent writers to the *same* document could still race the score. The supported indexing model is one writer per document (ARC ingestion is document-sequential); cross-document concurrency — the case A1 fixed — is fully race-free.
- The Node domain model and the `chunk_ids`/`score` properties it exposes are unchanged; the `InMemoryGraphStore` keeps the list schema behind its mutex. ADR-0039, 0040, 0041, 0042 are unaffected.

## Rationale

- The milestone's purpose is "turn existing enrichment output into a measured retrieval signal". Every v1 scope decision (entities only, no relations, ≥0.90 floor, cross-doc merge, worker-level write) is the smallest set consistent with the measured ceiling; anything unmeasured (concepts, relations, aliasing) is excluded by the same rule that froze M4/M6 contracts.
- Persistence lifecycle ties to the diff plan so the graph never drifts from the vector collection: one source of truth for what is indexed.

## Consequences

- `internal/graph/store`: `GraphStore` seam gains what the Qdrant adapter needs (`DeleteByDocument`, `FindNodeByName` per research); `QdrantGraphStore` implements node-only semantics; edge methods return not-supported.
- `internal/indexing/worker`: `WithGraphStore` option; entity extraction from chunk metadata at the ≥0.90 floor; idempotent upsert; delete cleanup.
- The `GraphRetriever` hardcoded scaffold (`chk-1`/`creativity`, in-memory type-assert) is replaced by a real strategy — decision in ticket 05.
- Retrieval fusion path, benchmark dimension, and orchestration gate remain open tickets (06, 07, 08); the acceptance rule (entity-category ≥5% gain over dense, no >5% regression elsewhere, gate metrics preserved) is confirmed in ticket 07 before any fusion ships.
