# HTTP/SSE API v1 (arc-server) with a single streaming answer path

The tldraw Research Canvas frontend (`frontend/REQUIREMENTS.md` §6) needs a REST/SSE surface, and `cmd/arc-server` was a stub that built a mock composition and never listened. The legacy `StreamingAnswerEngine` seam (M5) ran its own pipeline that diverged from `AnswerEngine.Answer`: no evidence gate, no retrieval orchestrator, no Phase 2 verification, so a canvas answer could deviate from `arc ask` semantics — including the honest `no_evidence` abstention and the Jev-entailed `verified` status.

We decided to build the v1 HTTP surface on the single production pipeline. Streaming moved INTO `AnswerEngine`: `AnswerStream(ctx, query)` shares the private `prepare` step (analyze, orchestrator retrieval, no-sources abstention, context assembly, evidence gate with bounded retry) with `Answer`, then streams LLM tokens and a final verification chunk carrying the runtime verifier's decision — Jev Phase 2 included because the runtime composes it. The legacy `StreamingAnswerEngine` was deleted (its only caller was its own test). `Runtime.AnswerEngine()` now exposes the one composition path (`RetrieverForMode` + `buildAnswerEngine`) that both the CLI and the server use, and `Runtime.VectorStore()` exposes the shared store for the read-only HTTP surface.

The v1 surface is a Fiber app: `GET /healthz`, `GET /api/v1/spaces`, `GET /api/v1/spaces/:spaceId/documents`, `GET /api/v1/chunks/:id` (wildcard route, chunk ids embed slashes), and `GET /api/v1/qa/stream?q=&spaceId=&topK=` as SSE. Each `AnswerStreamChunk` is one `data: <json>\n\n` event flushed per event; the frontend's `AsyncIterable<AnswerStreamChunk>` contract maps 1:1 onto the Go JSON. CORS defaults to the Next.js dev origin (`http://localhost:3000`); bind is `ARC_HTTP_PORT`/`PORT`, default 8080.

Status: shipped and live-verified against the real corpus (The Creative Act, 197 points): spaces lists the default space, documents list the one document with profile-derived title/author, chunk-by-id returns the payload, and the stream emits token → verification (Semantic verdicts present, Jev ran) → done.

## Considered options

- **Extend the legacy StreamingAnswerEngine**: rejected — it bypasses the gate, orchestrator, and Phase 2; patching it into parity means duplicating the engine. One pipeline beats two near-identical ones (ADR-0033's seam discipline).
- **Second engine composition in arc-server**: rejected — `Runtime.AnswerEngine()` centralizes one composition path used by CLI and server, so behavior cannot drift between surfaces.
- **REST-token streaming (poll)**: rejected — the frontend contract and the existing stream chunk types are SSE-shaped; a poll loop adds state for nothing.
- **Real space model now**: rejected — indexing never sets `WorkspaceID`/`KnowledgeSpaceID`, so v1 folds everything into one default space and reads the metadata field when it appears. The API does not fake a model the index does not hold.

## Consequences

- `Answer` and `AnswerStream` share `prepare`; abstention text and `StatusNoEvidence` are single-source constants, so the sync and stream surfaces cannot drift.
- The stream endpoint does not apply the frozen retrieval `MinScore` (the CLI's `arc ask` does); v1 exposes only `q`/`spaceId`/`topK` per the frontend contract, and the evidence gate remains the honesty floor. A `minScore` parameter can join later without contract changes.
- `DocumentSummary` carries `document_id` and `chunk_count` for sure; `title`/`author` come from the profile point's deterministic line format when present (the creative-act profile currently yields the known wrong-title enrichment quirk, tracked separately); `page_count`/`language` are omitted (no payload source).
- Chunk payloads carry no parent/child hierarchy links, so the chunk shape omits them at this seam.
- `github.com/gofiber/fiber/v2` joins the module (the stub never imported it; CONTEXT.md's Fiber claim becomes true).
- Live evidence: with qdrant reachable, `ListPoints` propagates failures; a closed qdrant is a 500, not a silent empty list.
## Amendment: document-scoped stream queries

The tldraw canvas drops research documents onto a source node and asks a question against exactly those documents. `GET /api/v1/qa/stream` now accepts `documentIds`, a comma-separated list of document ids (empty/absent keeps the unfiltered stream), and sets `MetadataFilter.DocumentIDs` on the retrieval query. Given together with `spaceId`, both fields land on the same `MetadataFilter` and AND on every retrieval leg. A list matching nothing is not an error: retrieval returns no sources and the engine streams its normal `no_evidence` verdict. Ids are parsed with a pure helper (`parseDocumentIDs`) at the HTTP boundary, so the filter never sees untrimmed or empty entries.

GET query parameters stay the v1 transport even though the list can reach tens of ids, which is comfortably inside URL length limits. If a canvas ever needs hundreds of ids or repeated filters, a POST JSON body can join additively: SSE over POST is already exercised by the upload endpoint (ADR-0052), so the event contract does not change.

Two retrieval legs previously dropped part of the filter. The document-overview retriever listed profile points with `DocumentIDs` only, so a space-scoped overview leaked every space's points; it now forwards both fields. The graph retriever applied its document-scope skip in the results loop but ignored `KnowledgeSpaceID`; the skip now mirrors the `DocumentIDs` check against chunk payload metadata, matching the Qdrant adapter's AND semantics. The offline in-memory store double gained the same `KnowledgeSpaceID`/`WorkspaceID` match checks so the space leg is actually exercised in tests.

## Amendment: documentIds as repeated parameters (2026-corpus comma ids)

document ids are derived from filenames and legitimately contain commas (`Dave Gray - Liminal Thinking ... (2016, Rosenfeld Media) - libgen.li`). The comma-split parser corrupted them (measured: a comma-containing id always ended as `no_evidence` because no split part matched). `documentIds` is now read as repeated query parameters via `QueryArgs().PeekMulti`, one complete id per value, with no splitting or trimming. The client sends each id with `URLSearchParams.append`, so commas, parentheses, braces, quotes, and spaces round-trip through URL encoding. Zero-length values are dropped; absent/empty input keeps the unfiltered behavior. Filename sanitization (stripping commas, braces, emoji) was considered and rejected for v1: it changes existing ids and forces a corpus re-index, it is lossy, and it risks id collisions (`A, B - X` and `A B - X` folding together). The id is an internal identity key; display naming belongs to the title field. Re-uploading a file under the SAME filename keeps the same id and updates via the diff worker (no duplicate); the same file under two different filenames produces two ids with independent points, which is the filename-based identity contract.
