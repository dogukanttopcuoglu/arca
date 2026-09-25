# Browser document upload with streaming ingest and deletion

Before this ADR, ingesting a PDF meant running `arc inspect <file.pdf>` in a terminal: the CLI read the file, ran the real inspection pipeline (Firecrawl extraction, semantic chunking, enrichment), and indexed through the differential worker. The tldraw Research Canvas frontend (`frontend/REQUIREMENTS.md` §6) has no CLI access; it needs to upload a PDF from the browser and watch a 1-3 minute job progress instead of staring at a silent spinner, then be able to remove the document again.

We decided to add two v1 endpoints to arc-server that reuse the CLI's exact pipeline rather than building a parallel ingest path:

- `POST /api/v1/documents` (multipart/form-data; `file` required, `spaceId` optional) ingests one PDF and streams the ingest phases as SSE events. `document_id` is derived with the CLI's byte-identical rule — `filepath.Base(filename without extension)` — so `arc inspect book.pdf` and a browser upload of `book.pdf` address the same document. Only `.pdf` (case-insensitive) is accepted; everything else is an in-stream error event.
- `DELETE /api/v1/documents/:documentId` removes the document's vector points and, when the entity graph store is attached, its graph node evidence (ADR-0038). An existence check runs before the delete: unknown ids are 404, so a re-delete cannot be mistaken for a successful removal.

The pipeline is exposed through two new runtime seams — `Runtime.Inspect(ctx, docID, data)` wrapping the inspector with a `bytes.Reader`, and `Runtime.Index(ctx, docID, title, chunks, meta)` wrapping `ExecuteSync` — and `App.RunInspect` was refactored onto them, so CLI and server share one implementation path and cannot drift. Deletion rides `Runtime.DeleteDocument`.

## Considered options

- **Upload-then-job (REST poll)**: rejected — the frontend contract is SSE-shaped, and a poll loop adds job state and a second status model for nothing. Streaming the phases reuses the qa/stream convention (one `data: <json>\n\n` event per phase, flushed per event).
- **Server-side document id (UUID)**: rejected — the CLI's base-name rule is the existing identity contract; a different server rule would make the same file indexable twice under two ids, and re-upload diff updates (no duplicates) depend on one deterministic id.
- **Duplicate the ingest pipeline in arc-server**: rejected — the CLI's inspect/index path is battle-tested and already handles diff updates; a second composition invites divergence (the same trap ADR-0033 and ADR-0051 warn about).

## Consequences

- Phase contract: `{"type":"phase","phase":"parse"}`, then `{"type":"phase","phase":"chunk","chunk_count":N}`, then `{"type":"phase","phase":"embed"}`, then `{"type":"phase","phase":"indexed","indexed":I,"skipped":S}`, terminated by `{"type":"done","document":{"document_id":...,"title":...,"chunk_count":...,"status":"success"|"partial_success","indexed":I,"skipped":S,"deleted":D,"warning_count":W}}`. Chunk count and status come from the inspection result; indexed/skipped/deleted come from the completed `IndexingJob`.
- Error contract: a missing `file` field, an unparseable form, or an unsupported `spaceId` is a plain 400 JSON before the stream. Everything after the stream starts is an in-stream `{"type":"error","error":"<verbatim message>"}` event: unsupported extension, inspection failure (including the aggregate diagnostics errors, mirroring `RunInspect`), indexing failure, or a failed diagnostics status. The browser never mistakes a broken ingest for a connection loss.
- Re-upload of the same `document_id` is an update, not a duplicate: the diff worker skips unchanged chunks (indexed 0, skipped >= previous count) and the documents listing still shows exactly one document.
- `spaceId` is accepted and validated (empty or `default` ok) but not persisted: v1 indexing never sets the space field, so the document always lands in the default space. The API does not claim to store what the index does not hold (the same restraint as ADR-0051's space model).
- The Fiber app raises `BodyLimit` to 100MiB so multi-megabyte books pass the previous 4MiB default.
- CORS now allows POST and DELETE and the `Content-Type` header so the browser can send multipart preflights.
- Tests run the real pipeline fully offline: an httptest stub stands in for Firecrawl (exactly how the CLI tests already do it), mock embeddings fill the in-memory store, and the assertion covers the event sequence, the done summary, listing visibility, re-upload dedup, error cases, deletion, and CORS preflight. No `.scratch` corpus is read.