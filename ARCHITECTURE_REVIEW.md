# ARC PDF Inspector — Technical Architecture Review

## 1. Project Purpose & Overall Architecture

ARC is a Go knowledge-ingestion platform. **PDF Inspector** is the entry-point stage: it turns raw PDFs into a canonical, versioned, serializable intermediate representation — `PDFInspectionResult` — that downstream RAG / vector-indexing / Knowledge Graph stages are supposed to consume.

The architecture is a **layered pipeline with hexagonal intent**:

- **Polyglot boundary**: PDF parsing/OCR/Markdown conversion is delegated to a Node.js **Firecrawl microservice** (`services/firecrawl/`) over HTTP, per ADR-0001/0002.
- **Go core** (`internal/pdfinspector/`): semantic tree reconstruction → hierarchical semantic chunking → asset extraction → diagnostics/resiliency → enrichment pass pipeline.
- **Delivery adapters** (`cmd/`): `arc` (CLI), `arc-server` (REST/SSE), `arc-mcp` (MCP server), `demo` (batch runner).
- **Future layers** (`internal/qa`, `retrieval`, `indexing`, `graph`, `agent`, `security`, `llm`): declared as seams with demo-grade/partial implementations.

Reality check vs. documentation: the README and ADR-0006 claim **Fiber**; there is **no Fiber dependency anywhere** (`go.mod` has only `fasthttp`, `viper`, `zap`, `testify`). No HTTP server exists at all. The stack actually used is `fasthttp` (client only).

## 2. Main Packages / Modules & Responsibilities

| Package | Responsibility | Status |
|---|---|---|
| `internal/pdfinspector/model` | Canonical domain (`PDFInspectionResult`, `KnowledgeChunk`, `SemanticTree`, assets, enrichment models), JSON versioning/nil-slice guarantees, `Validate()`, `DeepCopy` | **Mature, well-tested** |
| `internal/pdfinspector/firecrawl` | fasthttp HTTP client, retry/backoff, error mapping → `SERVICE_UNAVAILABLE` | Mature |
| `internal/pdfinspector/semantic` | Markdown/JSON → `SemanticTree` via heading-stack reconstruction | Mature but page-tracking fragile |
| `internal/pdfinspector/chunking` | Block parser + hierarchical chunk builder, token sizing, hashes/fingerprints, warning collector | Mature core, several defects (see §7) |
| `internal/pdfinspector/assets` | Table/figure/code/equation/citation sub-extractors + page/section/chunk resolvers + ordering | Mature |
| `internal/pdfinspector/diagnostics` | Fail-fast PDF validation, error mapping, diagnostics aggregation, graceful degradation | Mature |
| `internal/pdfinspector/enrichment` | Compiler-pass pipeline (language, chunk stats, title/author, page resolution, entity/keyword/concept/relation/summary) | **Mostly complete, partially wired** |
| `internal/pdfinspector/config` / `logger` | Viper env config / global zap | Config mostly **dead** |
| `internal/qa`, `retrieval`, `indexing`, `graph`, `agent`, `security`, `llm` | Downstream RAG/graph/agent seams | Seams + **mocks/stubs**, not wired |
| `cmd/*` | Entry points | `demo` real; `arc`/`arc-server`/`arc-mcp` are **stubs that `os.Exit(0)`** |
| `services/firecrawl` | Express PDF extraction service | Functional but heuristic fallback |

## 3. Dependency Flow Between Packages

```
cmd/demo ──► inspector ──► firecrawl ──► (HTTP) services/firecrawl
             │  ├─► semantic ──► model
             │  ├─► chunking ──► model
             │  ├─► assets   ──► model
             │  ├─► diagnostics ──► model
             │  └─► enrichment ──► model
             └─► model

model ← consumed by: indexing (diff/worker), and (via VectorMetadata) qa/retrieval
```

- **Only `indexing/` imports `pdfinspector/model`** (`KnowledgeChunk`, `ContentType`). Everything else downstream is disconnected.
- `qa/`, `retrieval/`, `graph/`, `agent/`, `security/`, `llm/` never import pdfinspector — they operate on flattened `VectorMetadata`/`SearchResult`.
- Dependency direction within pdfinspector is clean and one-way (`model` at the bottom, `inspector` on top). No cycles.

## 4. Complete Runtime Flow

The only truly exercised path is `cmd/demo` (and tests):

1. **PDF input** — read into memory (`strings.Reader`/`os.ReadFile`).
2. **Fail-fast validation** — `diagnostics.ValidatePDFStream`: checks `%PDF-` header + `/Encrypt` marker; returns `ErrInvalidDocument` / `ErrEncryptedDocument` with a `failed` diagnostics payload.
3. **Firecrawl extraction** — `HTTPClient.ExtractPDF` POSTs the raw bytes as `application/pdf`, retries 5xx with exponential backoff (default 3 retries, 30s timeout), maps 4xx fail-fast.
4. **Raw result** — `RawExtractionResult{markdown, json_layout, metadata, ocr_applied}` returned.
5. **Semantic tree** — `semantic.Processor.ProcessExtraction` prefers `json_layout.nodes` (dead path — the service never emits `nodes`), otherwise parses Markdown headings into a `SemanticTree` using a level-stack, tracking pages from `<!-- page:N -->` / `\f` / `[page N]` markers (rarely present, see §7).
6. **Chunking** — `chunking.Engine.ChunkDocument` parses Markdown into `SemanticBlock`s (paragraph/table/code/equation/figure/list) and groups them by `section_path` into `KnowledgeChunk`s with parent/child links, neighbor links, source offsets, citations, hashes, fingerprints. **DocumentID hardcoded to `doc-1`.**
7. **Document content** — `buildDocumentContent` builds `PageMap` from `json_layout.pages` (fallback: single page 1).
8. **Asset extraction** — five sub-extractors run over the full markdown; then page/section/chunk resolvers enrich each asset; assets sorted by offset into `Assets.Ordered`; stats computed.
9. **Metadata build** — `buildDocumentMetadata` reads title/author/fonts/pageCount/etc. from `raw.Metadata`, falls back to first root heading, then "Untitled Document".
10. **Enrichment** — `CompositeEnricher.ExecutePasses` runs 9 passes against the metadata/tree/pageMap/chunks (language → chunk stats → title/author → page resolution → entities → keywords → concepts → relations → summaries). **Uses `context.Background()`, discards pass warnings.**
11. **Diagnostics** — aggregated; auto-degrades `success` → `partial_success` if any warnings or skipped pages.
12. **Output** — `PDFInspectionResult` assembled, `Validate()` optional, serialized to JSON (demo writes `inspection_result.json`).

**The full ingestion chain stops there.** Nothing feeds `PDFInspectionResult.Chunks` into `IndexingWorker`; `cmd/arc inspect` doesn't even call the inspector.

## 5. Current Implementation Maturity

**Completed (solid):**
- Full `PDFInspectionResult` contract with version stamping, nil-slice guarantees, `Validate()`, `DeepCopy`.
- Firecrawl client with backoff/retry + test seams; fail-fast PDF validation; error mapping.
- Semantic tree reconstruction, hierarchical chunk builder, asset extraction, diagnostics aggregator.
- Enrichment pass architecture (capability contracts, CompositeEnricher) with rule-based + hybrid (GLiNER) providers; quality benchmark/behavior-contract tests.
- JSON Schema (v1) and spec doc; docker-compose for the Firecrawl service.

**Partially implemented:**
- **Page provenance**: chunk `page_numbers` are effectively broken for the real service (all default to `[1]` — visible in `inspection_result.json`: `max page in chunks = 1`); enrichment fixes the *semantic tree* pages but not chunks.
- **Enrichment wiring**: `MetadataConsistencyPass` exists + `CapabilityMetadataConsistency` added to `pass.go` (uncommitted) but the pass is **not in the `Enrich` pipeline**; warnings are discarded (§7).
- **`cmd/arc inspect`**: returns a hardcoded string; no inspector call.

**Missing / stubbed:**
- No Fiber/HTTP server (`arc-server` prints "listening on :8080" then `os.Exit(0)`; `arc-mcp` likewise).
- No sparse/BM25 retriever, no real embedding/LLM providers, no Qdrant/PgVector store, no real graph store; `GraphRetriever` hardcodes `chk-1`/`creativity`; `AgentEngine` uses a hardcoded plan; `SecurityContext` is orphaned; `EntailmentChecker` never invoked; `QAJobWorker` has no execution loop; `security` package is dead code.
- Enrichment `LLM`/`TOC` resolvers and `hybrid` keyword/concept/relation/summary extractors (only rule-based + hybrid entity exist).

## 6. Architectural Decisions Review

**ADR alignment:**
- ADR-0001/0002/0003/0004/0005 — **implemented**, but with deviations: no circuit breaker, no page batching/streaming, absolute token max not enforced (§7).
- ADR-0006 (Go + **Fiber**) — **not implemented**; Fiber absent, no server.
- ADR-0007 (semantic page resolution) — partially implemented; the *diagnostics-warning* requirement is dropped (§7).
- ADR-0019→0025 — implemented, but ADR-0025's documented frozen pass order (`Language→Title→Keyword→Entity→Concept→Relation→Summary`) differs from code (Keyword correctly comes *after* Entity, which needs Entities). Doc/code drift.
- ADR-0008→0018 (indexing/QA/graph/agent) — seams declared, mostly **mock/stub** implementations, not wired to ingestion.

**Clean Architecture / dependency direction:** Good. `internal/` core is framework-agnostic; adapters in `cmd/`. One-way imports, no cycles. The main violation is **mock/hardcoded logic in production paths** (worker hardcodes `MockProvider`; streaming engine falls back to a mock string; MCP tools return canned results), which blurs the seam-vs-implementation line.

**Separation of responsibilities:** The model/pipeline layering is clean. Weak spots: `DefaultEnricher.Enrich` hardcodes the full pass list (pipeline composition isn't injectable); rule-based extractors contain **test-fixture data** (hardcoded "Rick Rubin", "Mustafa Kemal Atatürk", "Ankara" person/location patterns in `entity_extractor.go`), and `TitleResolver`/`genericHeadings` carry book-specific entries ("78 areas of thought").

## 7. Bugs, Hidden Assumptions, Tech Debt, Risks

### Possible bugs
1. **`document_id` is always `doc-1`.** Inspector calls `chunking.NewEngine()` with no `WithDocumentID`; default is `doc-1`. Every chunk across every document shares `document_id: "doc-1"` and chunk IDs collide. Breaks multi-document RAG.
2. **Chunk page provenance broken for the real service.** The firecrawl service emits page info only via `json_layout.pages`; the Go parser tracks pages only from `<!-- page:N -->`/`\f`/`[page N]` markers the service never emits → all chunks land on page 1 (confirmed in the 248-page artifact).
3. **Enrichment warnings lost.** `PageResolutionPass` does `_ = EnrichSemanticTree(...)`; `CompositeEnricher` never appends pass warnings to `report.Warnings` → ADR-0007's "resolution failures produce diagnostics warnings" is unimplemented.
4. **Citation IDs collide.** Parser `extractCitations` uses a per-block counter (`cit-1`, `cit-2`…) so citation IDs are not globally unique; asset-level citations use a separate sequential namespace.
5. **Absolute token max not enforced.** 14/196 real chunks exceed 1200 tokens (up to 2209). A single oversized paragraph is emitted intact with `is_oversized` — by design, but contradicts "absolute max". No hard-split fallback exists.
6. **Parent chunks duplicate content.** For multi-child sections a parent chunk concatenates *all* child content → embedded duplicates inflate storage and retrieval noise; neighbor links (`previous`/`next`) interleave parent/child chunks.
7. **`IndexingWorker` deletion mismatch.** `DiffPlan.DeletedPointIDs` holds point IDs but the worker deletes via `MetadataFilter{ChunkIDs: ...}` → deleted points likely never removed. Also worker hardcodes `"MockProvider"`/`"mock-model-v1"`.
8. **`ValidatePDFStream` false positives.** `containsEncryptionMarker` can flag non-encrypted docs containing `/Encrypt` in content within first 1024 bytes; no cross-check of the trailer. Conversely, encryption is only detected in the first 1024 bytes.
9. **`DenseRetriever.Retrieve` never populates `ContentMarkdown`**, so QA context building always falls back to a stub.
10. **Zero-time dates.** `creationDate`/`modificationDate` never populated from extraction → serializes as `0001-01-01T00:00:00Z` (empty time.Time defeats `omitempty`).

### Hidden assumptions
- Firecrawl markdown contains page markers — **false** with the bundled service.
- GLiNER at `localhost:8088` — default in `NewEnricher()`; in docker-compose **no GLiNER service exists**, so every inspection pays per-chunk HTTP timeout overhead (sequential, up to 3s each).
- `json_layout.nodes` is populated — **false**; the whole `processJSONNodes` path is dead against the real service.
- `PDFInspectionResult` is deterministic — true, but the enrichment hybrid extractor consults a network service, and `Enrich` uses `context.Background()` (uncancellable).

### Technical debt
- **Test data in production code**: hardcoded "Rick Rubin"/"Atatürk"/"Ankara" heuristics; book-specific generic-heading entries.
- **Dead config**: `MaxDocumentSize` (100MB) and `MaxPageCount` never enforced; `HTTP_TIMEOUT`/`MAX_RETRIES` loaded but never passed to the client. No size guard → unbounded in-memory buffering (PDF read once in `ValidatePDFStream`, again in `ExtractPDF`).
- Orphaned seams: `SecurityContext`, `QAJobWorker`, `EntailmentChecker`, `CapabilityMetadataConsistency` (unwired), `MetadataConsistencyPass` (uncommitted + unwired).
- Duplicate page-tracking logic in three places (semantic processor, chunk parser, `DefaultPageResolver` substring search) with subtly different marker regexes.

### Scalability risks
- **Whole-document buffering**: no streaming; multi-GB PDFs risk OOM. Firecrawl `express.raw` also caps at 50MB.
- **O(n) page-map substring resolution** in `DefaultPageResolver`.
- **Sequential per-chunk GLiNER calls** in the default enricher.
- Regex-heavy full-document scans × 5 asset extractors.
- Serialization of a 1.2MB result per document with no pagination/streaming at the API layer.

### Improvement priorities
1. Thread the real `document_id` into the chunker; make it a required input.
2. Reconcile page provenance: have the firecrawl service emit page markers (or consume `json_layout.pages` in the chunk parser), then make enrichment fix chunk pages too.
3. Wire `MetadataConsistencyPass`, surface enrichment warnings into diagnostics, and thread `ctx` through `Enricher`.
4. Enforce size caps and configure the client from `Config`.
5. Add a real hard-split fallback for oversized atomic blocks (sentence-level) and skip parent-chunk duplication unless explicitly needed.
6. Remove fixture data from extractors; make `Enrich` pass list injectable.

## 8. Special-Attention Areas

**PDFInspectionResult contract** — strong: versioned, validated, deep-copied, schema-backed. Gaps: no date parsing, enrichment models (keyword/entity/concept/relation/summary) exist in Go but are **absent from the v1 JSON Schema**, so the "canonical contract" and the schema have drifted.

**Semantic tree resolution** — heading-stack logic is correct; the weakness is page derivation (marker-dependent) and the ADR-0007 fallback chain only partially wired (chunk fallback is unreliable because chunk pages are wrong).

**Chunking strategy** — ADR-0004 philosophy implemented; structural issues are the hardcoded doc ID, non-absolute absolute-max, parent duplication, and heading text excluded from chunk content (downstream RAG relies solely on `section_path`).

**Diagnostics/resiliency** — fail-fast + partial_success + skipped pages work well and are tested; missing: circuit breaker, per-stage error scoping beyond warnings, enrichment warnings, and actual config-driven limits.

**Enrichment architecture** — best-designed part (compiler-pass + capability contracts + strategy seams + hybrid provider with graceful fallback). Impediments: non-injectable pass list, dead `MetadataConsistencyPass`, network dependency in default constructor, discarded warnings, doc/code ordering drift.

**RAG / Knowledge Graph integration** — the output contract is ready (chunks carry parent/child links, section paths, citations; enrichment produces entities/concepts/relations), but **nothing downstream consumes it**: `indexing` is the only importer and is never invoked; graph is disconnected from enrichment output; retrieval/QA/agent are mock-grade. The pipeline currently ends at JSON serialization.

---

**Bottom line:** the pdfinspector bounded context is a well-factored, tested core that implements most of ADR-0001–0007 and 0019–0025, but carries one critical data-integrity bug (hardcoded `doc-1`, broken chunk page provenance) and is disconnected from the downstream RAG/graph/indexing layers, several of which are presentation-grade stubs. The highest-value next steps are fixing document-ID/page provenance, wiring enrichment warnings + `MetadataConsistencyPass`, enforcing config limits, and building a real server/CLI that runs the actual pipeline end-to-end into indexing.

---

*Review date: 2026-08-02. Analyzed at commit `fc2ae0b` with two uncommitted enrichment changes (`pass.go` `CapabilityMetadataConsistency`; new `metadata_consistency_pass.go`) present in the working tree.*
