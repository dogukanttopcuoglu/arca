# ARC Engineering Handbook

> How ARC works, from first principles. Written as a day-one guide for a backend engineer who knows Go but has never seen ARC.
> Every section follows the same shape: **Problem → Naive Solution → ARC Solution → Worked Example → Internal Data → Trade-offs → Why This Matters → What Would Break Without It → Next Stop**.

---

# Part 0 — The Big Picture

## Problem

A human being reads a book. She opens it, skims the table of contents, jumps to a chapter, notices a table, remembers a name, cross-references a footnote, and an hour later can answer questions like *"what did the author say about X?"* or *"how does the author compare A and B?"* — citing roughly which page she read it on.

Software cannot do that. A PDF is a pile of glyphs; a `.docx` is a zip of XML. To answer questions about a document, a machine must:

1. **Read** the document (extract its text, structure, tables, figures).
2. **Chunk** it into self-contained pieces small enough for an LLM's context window.
3. **Index** those pieces so they can be found by *meaning*, not just exact words.
4. **Retrieve** the few pieces relevant to a question.
5. **Answer** the question grounded *only* in those pieces, with citations to the original pages.

ARC ("Knowledge OS") is that pipeline, built for books: real, page-numbered, multi-hundred-page documents — not chat logs or web pages.

The founding constraint that shapes every decision in this handbook: **ARC cannot afford to be wrong in a way that is invisible.** If retrieval silently degrades, answers become confidently wrong. So ARC treats *measurement* as a first-class system: every architectural change must pass a benchmark gate (Gold Set + Corpus Fingerprint) before it reaches production. If a change does not measurably help, it does not ship. This is the single most important thing to understand about this codebase — it is why there are 46 ADRs and why frozen constants exist.

## Naive Solution

An inexperienced engineer builds "PDF → text → split into 500-token windows → embed → cosine search → stuff into prompt". It works on demos and fails on real books:

- **Naive splitting breaks meaning.** A window boundary lands mid-argument; the retrieved chunk is 500 tokens of two unrelated paragraphs. Answer quality collapses silently.
- **Naive reading loses structure.** Headings, tables, multi-column layouts, footnotes — the things a human uses to navigate — are destroyed. *"Chapter 12, What Reasonable Conclusions Are Possible?"* becomes unsearchable.
- **Naive embedding loses citations.** "Page 157" is a *fact* about where a claim lives. If you throw away page numbers, the answer cannot cite anything.
- **Naive re-indexing is wasteful and stale.** Re-uploading a fixed book re-embeds 100% of chunks even when 95% are unchanged.
- **Naive RAG invents answers.** With no evidence gate, the LLM happily answers a question the corpus cannot answer, citing nothing.

## ARC Solution — the architecture map

ARC is a pipeline of seams. Each box is a module behind an interface, and each arrow is data flowing through a contract:

```
                 ┌──────────────────────────────────────────────────┐
                 │                  The Book (PDF)                   │
                 └──────────────────────┬───────────────────────────┘
                                        │
                 ┌──────────────────────▼───────────────────────────┐
                 │  Firecrawl PDF Service (Node/TS, Docker)         │  ADR-0001/0002
                 │  raw parsing, OCR detection, Markdown + layout   │
                 └──────────────────────┬───────────────────────────┘
                                        │ RawExtractionResult{Markdown, JSONLayout, OCRApplied}
                 ┌──────────────────────▼───────────────────────────┐
                 │  PDF Inspector (Go)                              │  ADR-0003
                 │  semantic tree → chunks → assets → enrichment    │
                 │  → PDFInspectionResult                           │
                 └──────────────────────┬───────────────────────────┘
                                        │ PDFInspectionResult{Document, SemanticTree, Content,
                                        │ Chunks, Assets, Diagnostics}
                 ┌──────────────────────▼───────────────────────────┐
                 │  Indexing Worker (Go)                            │  ADR-0008
                 │  differential diff → embed → upsert (Qdrant)     │
                 └──────────────────────┬───────────────────────────┘
                                        │ points: dense (+sparse) + payload metadata
                 ┌──────────────────────▼───────────────────────────┐
                 │  Retrieval (Go)                                  │  ADR-0028/0036/0041
                 │  dense / sparse / hybrid RRF / graph fusion      │
                 └──────────────────────┬───────────────────────────┘
                                        │ []SearchResult (TopK, ranked, scored)
                 ┌──────────────────────▼───────────────────────────┐
                 │  AnswerEngine (Go)                               │  ADR-0030..0037, 0042
                 │  analyze → orchestrate → retrieve → context      │
                 │  → evidence gate → prompt → LLM → verify         │
                 └──────────────────────┬───────────────────────────┘
                                        │ Answer{Text, Citations, Verification, Status}
                                        ▼
                                   User's answer
```

The two vertical slices of the system that never touch each other directly:

- **Ingestion** (top half): documents in → `PDFInspectionResult` → vector points. This path is *deterministic*: given the same PDF, it produces the same chunks, hashes, and metadata. Nothing in it calls an LLM (enrichment is rule-based by default).
- **Retrieval/QA** (bottom half): query in → ranked chunks → grounded answer. This path is *stateless with respect to documents*: it only reads the vector store.

The bridge between them is the **vector point payload**: every chunk is persisted with a rich, versioned metadata envelope (`VectorMetadata`), and that envelope is what makes filters, differential indexing, and citation-carrying answers possible.

## The worked example we will follow

This handbook follows **one document through the entire pipeline**: *The Creative Act: A Way of Being* by Rick Rubin (`rick-rubin.pdf`, 248 pages). It is real — it lives in `.scratch/m5-corpus/pdfs/`, and its real inspection output lives in `.scratch/m5-corpus/results/rick-rubin.json` (196 chunks, 248 pages). We will meet its data at every stage.

Its journey, which each Part of this handbook expands into a full walkthrough:

```
rick-rubin.pdf
   │  248 pages, text-based
   ▼
[Firecrawl service]  ──►  markdown + json_layout (pages/nodes) + metadata
   ▼
[PDF Inspector]
   ├─ Semantic tree: 98 root nodes ("Robert Henri", "78 Areas of Thought", ...)
   ├─ Chunks: 196 chunks, ids like rick-rubin/tuning-in/002
   ├─ Assets: 3 citations (attribution, inline, ...)
   ├─ Enrichment: title "Robert Henri"→(fallback), keywords, entities (Rick Rubin), relations (founded_by Def Jam)
   └─ Diagnostics: partial_success (1 warning: preamble text)
   ▼
[Indexing Worker]
   ├─ diff: first index → all 196 chunks NEW → embed 196
   └─ upsert: 196 points into Qdrant "arca_chunks" (dense 768-dim + sparse BM25)
   ▼
[Retrieval]   "What does the book say about Rick Rubin?"  (entity intent)
   ├─ UseGraph gate opens → GraphFusionRetriever (dense + entity graph, w=1.0)
   └─ top-5 chunks ranked
   ▼
[AnswerEngine]
   ├─ context window: [Ref 1]... [Ref 5] with page numbers
   ├─ evidence gate: supported?
   └─ LLM answer with citations → verified
   ▼
"Rick Rubin co-founded Def Jam Recordings with Russell Simmons [Ref 2] (p. 23, Tuning In)"
```

## The design principles (read these once, they explain every ADR)

1. **Measurement decides, assumption never does.** Every retrieval-affecting change lands behind a benchmark gate: Gold Set queries with declared expected chunks, Corpus Fingerprint hard-fail, frozen acceptance thresholds (ADR-0027). M4 (fusion), M6 (orchestration), M7 (graph), M8 (reranking) all followed the same arc: *grill → ADR → benchmark → freeze or reject*. Rejection is a valid outcome and code is written so rejection is trivially reversible (a config value, not a fork).
2. **Deep modules behind seams.** Every subsystem is an interface (`firecrawl.Client`, `EmbeddingProvider`, `VectorStore`, `Retriever`, `Reranker`, `EvidenceGate`, `LLMProvider`, ...). The point is *testability* and *swappability* — but also *discipline*: the seam documents the contract, and the contract is what benchmarks measure.
3. **Determinism is a contract.** Scores tie-break by ChunkID ASC everywhere. RRF uses a frozen k=60. Term IDs come from a sorted vocabulary. Bootstrapped confidence intervals use a fixed seed. If a benchmark cannot be reproduced, it is worthless.
4. **Graceful degradation, not all-or-nothing.** A corrupt page, a failed OCR, a missing table — none of these fail the pipeline. They produce `partial_success` diagnostics with warnings and skipped pages. Only structural failure (encrypted, invalid PDF) fails the inspection.
5. **Frozen over tuned.** Weights (`FusionPolicy`, `GraphFusionConfig.GraphWeight`), thresholds (`RETRIEVAL_MIN_SCORE=0.6`), budgets (`ComparisonTopK=8`), constants (RRF k=60) are all *frozen artifacts of benchmark calibration*. Runtime code never tunes them.
6. **Non-destructive enrichment.** Everything that enriches data (titles, pages, entities, keywords) can fail and leave diagnostics behind — it must never fail the pipeline.

---

# Part 1 — Document Ingestion: The Firecrawl Boundary

## Problem

ARC must turn a PDF into structured, citable knowledge. The first mile is raw extraction: parse the file format (PDF is notoriously hostile: embedded fonts with broken encodings, multi-column layouts, tables drawn with vector graphics, scanned pages with no text at all), detect which pages are scanned (OCR territory) and which are text, and produce *clean Markdown* plus a *layout description* that downstream Go code can consume.

Building a robust PDF parser is a multi-year project (font CMaps, CID encoding, reading-order reconstruction, table detection...). ARC is a knowledge OS, not a PDF parser vendor.

## Naive Solution

Write a Go PDF parser in-house (`pdfcpu` or `unipdf`-style), extract text per page, dump it. Why it fails:

- **Fonts lie.** Many books (especially older or scanned-through-OCR pipelines) use fonts with broken ToUnicode maps; naive extraction produces garbage or nothing. You then can't tell whether the *parser* failed or the *book* is unreadable.
- **Scanned pages have zero text.** A 248-page book with 40 scanned pages silently loses 16% of its content with no diagnostic — unless you detect it.
- **Layout is content.** "Page 12 is a two-column newspaper-style spread" and "this is a table with 4 columns" are facts retrieval needs; naive text dumps destroy them.
- **You lock yourself into your own bugs.** Every parser defect is now yours to maintain, and there is no upstream community fixing it.

## ARC Solution (ADR-0001, ADR-0002, ADR-0006)

**Buy the parser, own the analysis.** ARC runs Firecrawl's open-source PDF pipeline as a **dedicated Node.js/TypeScript microservice in Docker** (ADR-0002) — polyglot isolation: the heavy lifting (parsing, OCR detection, layout reconstruction, Markdown conversion) lives in a swappable engine behind a strict HTTP contract. ARC's Go code only ever sees the *contract*:

```go
// internal/pdfinspector/firecrawl/client.go
type Client interface {
	ExtractPDF(ctx context.Context, r io.Reader) (*model.RawExtractionResult, error)
}
```

The client (`HTTPClient`) is deliberately boring and robust:

- `POST {baseURL}/v1/extract` with raw PDF bytes (`application/pdf`), fasthttp pooled requests.
- Default timeout 30s (shrunk to the context deadline if one exists).
- **Retry policy**: 4xx → fail fast (the request is wrong, retrying won't fix it); 5xx → exponential backoff (100ms, 200ms, 400ms), up to 3 retries, then a typed `ErrServiceUnavailable`. The retry count is stamped into `metadata["retry_count"]` so diagnostics can report it.
- Config via env: `FIRECRAWL_BASE_URL` (default `http://localhost:3002`), `HTTP_TIMEOUT`, `MAX_RETRIES`, `MAX_DOCUMENT_SIZE_BYTES` (100MB), `MAX_PAGE_COUNT` (1000).

The contract the service returns is deliberately *loose* — nested maps, not structs — because it is a wire format owned by the external service:

```go
// internal/pdfinspector/model/schema.go:40-46
type RawExtractionResult struct {
	Markdown   string                 `json:"markdown"`
	JSONLayout map[string]interface{} `json:"json_layout"`
	Metadata   map[string]interface{} `json:"metadata"`
	OCRApplied bool                   `json:"ocr_applied"`
}
```

Three payloads: the Markdown (primary), a structured layout JSON (secondary; the service's own nodes/pages view), and free-form metadata (title, author, fonts, page count...). **Go code must treat `JSONLayout` and `Metadata` as untrusted, possibly-absent, loosely-typed maps** — every read is a type assertion with a fallback.

## Worked Example

`arc inspect rick-rubin.pdf` fires `ExtractPDF`:

1. The client validates the stream **before** the network call (`ValidatePDFStream`): `%PDF-` must appear in the first 1024 bytes (`MaxHeaderCheckBytes`), and `/Encrypt` markers are detected → encrypted documents fail fast with `ErrEncryptedDocument` **without ever paying for an OCR run** (ADR-0005).
2. The PDF bytes are sent; the service returns (real, from `.scratch/m5-corpus/results/rick-rubin.json` provenance):

```json
{
  "markdown": "# The Creative Act: A Way of Being\nwritten by Rick Rubin\n\n...248 pages of markdown...",
  "json_layout": {
    "pages": [ { "page_number": 1, "markdown": "The object isn't to make art..." }, ... ],
    "nodes":  [ { "type": "heading", "page": 5, "level": 3, "text": "Robert Henri" }, ... ]
  },
  "metadata": {
    "title": "The Creative Act: A Way of Being",
    "author": "Rick Rubin",
    "page_count": 248,
    "pdf_type": "text-based",
    "retry_count": 0
  },
  "ocr_applied": false
}
```

3. If the service is down (5xx), the client returns `ErrServiceUnavailable` after ~700ms of backoff; `MapFirecrawlError` in diagnostics canonicalizes it, and the inspector returns `StatusFailed` diagnostics alongside the error — **never a nil result**. The CLI prints a clear failure; the document is not partially indexed.

## Internal Data — the seam's invariants

- `RawExtractionResult.Markdown` may be empty for scanned books (the service returns what it can).
- `JSONLayout["pages"]` is a `[]interface{}` of maps `{page_number, markdown|text}` — the *authoritative page layout* for downstream PageMap (more on this in Part 2).
- `JSONLayout["nodes"]` is a `[]interface{}` of maps `{type, page, level, text}` — the service's structured heading/content nodes; its presence switches the semantic processor to the JSON path.
- `Metadata["skipped_pages"]` / `Metadata["failed_pages"]` feed diagnostics; `retry_count` is stamped by our client.

## Trade-offs

| Choice | Cost |
|---|---|
| External service (Docker) | One more moving part; the service is outside the Go toolchain. Mitigated by the strict contract + retry/backoff. |
| Loose map types | Type-assertion noise and silent fallbacks. Mitigated by `MapFirecrawlError` canonicalization and strict downstream validation. |
| 30s timeout | Large books may time out; mitigated by retry and by the service being local (same Docker network). |
| Fail-fast on 4xx | If the service changes its contract (new required header), every call fails loudly — which is exactly what you want for a wire-format change. |

## Why This Matters

The extraction boundary is where *all* of ARC's content quality enters. Everything downstream — headings, pages, tables, chunks — is only as good as this Markdown. By isolating it behind `Client`, ARC can later swap the engine (e.g. a Rust pdf-inspector binary — see the M8-era research in `docs/research/rag-reranking-architectures.md` for the same buy-vs-build reasoning) **without touching any downstream code**, as long as the contract holds.

## What Would Break Without It

Without this boundary, PDF parsing defects leak into ARC as silent data corruption: garbled fonts become garbage chunks, scanned pages become missing knowledge, and *no diagnostic distinguishes "book is bad" from "our parser is bad"*. The fingerprint-based evaluation system (Part 6) would measure noise instead of retrieval quality.

## Next Stop

The raw result goes to the **PDF Inspector**, which is where a pile of Markdown becomes a semantic tree with page numbers — Part 2.

---

# Part 2 — The PDF Inspector: Semantic Tree, Page Mapping, Chunking

## Problem

The extraction service returns one long Markdown string and a loose JSON layout. ARC needs:

1. **A semantic tree** — "this is a section called *Tuning In*, it contains these subsections, and all of it starts around page 11" — because chunks inherit their section path and pages from it.
2. **Page numbers on every chunk** — citations need "page 23", not "somewhere in the document".
3. **Self-contained chunks** — 400–700 tokens each, never splitting a table, never merging two sections, with parent/child links so a question can zoom in (child) or out (parent).

The output is the versioned, serializable `PDFInspectionResult` (ADR-0003) — the canonical intermediate representation every downstream service consumes.

## Naive Solution

Split the Markdown on blank lines into "blocks", cut blocks into 500-token windows, done. Why it fails:

- **Window splitting destroys arguments.** A 1,200-token paragraph (real in this corpus — see the oversized chunk in the worked example below) gets chopped mid-sentence; retrieval then can't find the full argument.
- **No hierarchy.** "Tuning In" as a heading is meaningless unless it *contains* its paragraphs. A flat list cannot answer "what section is this chunk from?"
- **No pages.** Nothing in the split knows about pages. The first requirement of ARC — citable answers — dies immediately.
- **Tokens are guessed wrong.** A naive `len(text)/4` overcounts dense prose and undercounts lists; the 400–700 budget is silently violated.

## ARC Solution

The inspector (`internal/pdfinspector/inspector/inspector.go`) runs a fixed pipeline (`InspectPDF`, lines 83–244):

```
validate stream → ExtractPDF → ProcessExtraction (semantic tree)
→ buildDocumentContent (content + PageMap from json_layout.pages)
→ ChunkDocument (hierarchical semantic chunking)
→ ExtractAssetsWithContext (tables, figures, code, equations, citations)
→ buildDocumentMetadata → Enrich (title, language, entities, keywords...)
→ BuildDiagnostics (auto-degrade success→partial_success on warnings)
```

Two reconstruction paths feed the tree:

- **`processJSONNodes`** — used when `json_layout.nodes` is present: each node is `{type, page, level, text}`; `heading|header` creates a tree node, `paragraph|text|table|code|list` propagates its page to all open ancestors. Unknown types → warning.
- **`processMarkdown`** — the fallback and the workhorse: a stack-based heading walker over Markdown lines.

Both build the same shape:

```go
// internal/pdfinspector/model/semantic.go
type SemanticNode struct {
	ID          string         `json:"id"`          // "sec-1", "sec-2", ...
	Heading     string         `json:"heading"`
	Level       int            `json:"level"`       // 1..6
	PageNumbers []int          `json:"pageNumbers"` // accumulated from content beneath
	Children    []SemanticNode `json:"children,omitempty"`
}
type SemanticTree struct {
	RootNodes []SemanticNode `json:"rootNodes"`
}
```

### The markdown walker (worked example, real lines from rick-rubin)

Input line by line; three kinds of lines matter, matched by these exact regexes:

```go
pageMarkerRegex = regexp.MustCompile(`(?i)<!--\s*page:?\s*(\d+)\s*-->|(?i)^---\s*page\s*(\d+)\s*---|(?i)\[page\s*(\d+)\]`)
pageBreakRegex  = regexp.MustCompile(`(?i)<!--\s*pagebreak\s*-->`)
headingRegex    = regexp.MustCompile(`^(#{1,6})(\s+(.*)|$)`)
```

- A page marker line sets `currentPage`.
- A heading line pops the stack while `top.Level >= level`, then pushes a new node. **While the stack is open, every content line calls `addPage(currentPage)` on every node on the stack** — that is how ancestors accumulate `PageNumbers`.
- Non-heading content before the first heading → the `preamble` warning (this is the real `partial_success` cause in rick-rubin).
- No headings at all → synthetic root `"Document Overview"` (a fallback for unstructured books).

Real output (verbatim from `.scratch/m5-corpus/results/rick-rubin.json` — note this book has **no H1/H2**, so all roots are level 3):

```json
{ "id": "sec-1", "heading": "Robert Henri",       "level": 3, "pageNumbers": [5] }
{ "id": "sec-2", "heading": "78 Areas of Thought", "level": 3, "pageNumbers": [6] }
```

If the extraction service marked a level-skip (e.g. H3 under no H2), the walker emits `"heading level skipped: expected level %d before level %d..."` — a warning, not a failure (graceful degradation).

### PageMap — where pages really come from

The semantic walker's `currentPage` tracking is a *fallback*. The authoritative page layout is `json_layout.pages` from the extraction service, which the inspector turns into:

```go
// internal/pdfinspector/model/document.go
type PageMap struct {
	PageNumber int    `json:"pageNumber"`
	Markdown   string `json:"markdown"`
}
type DocumentContent struct {
	Markdown string    `json:"markdown"`
	PageMap  []PageMap `json:"pageMap"`
}
```

Real entry (page 5):

```json
{ "pageNumber": 5, "markdown": "The object isn\u2019t to make art, it\u2019s to be in that wonderful state which makes art inevitable.\n\n### Robert Henri\n" }
```

The PageMap drives *chunk-level* page resolution (next section). **This is the single most important wire-format dependency in the ingestion half of ARC**: if the extraction service ever stops producing `json_layout.pages`, chunk page numbers silently vanish (a fact that shaped the M8-era "can we swap pdf-inspector?" analysis: a replacement must synthesize this structure).

### Hierarchical Semantic Chunking (ADR-0004)

The chunker is a two-stage pipeline: `BlockParser` (Markdown → semantic blocks) then `ChunkBuilder` (blocks → chunk tree).

**Stage 1 — BlockParser.** Walks the Markdown with the *same* page-marker regexes plus content detectors, producing blocks:

```go
// internal/pdfinspector/chunking/block.go
type SemanticBlock struct {
	Kind            SemanticKind      // paragraph|table|code|list|equation|figure
	HeadingLevel    int
	SectionPath     string            // headings joined with " > "
	Markdown        string
	PageNumbers     []int
	SourceOffsets   SourceOffset       // {start_char, end_char}
	Citations       []Citation
	SemanticCategory SemanticCategory // narrative|definition|procedure|reference|example|warning|code|table|equation|figure
}
```

Detection order per line: fenced code → headings (rebuild `sectionStack`, `SectionPath = "A > B > C"`) → equations (`$$...$$`, `\[...\]`) → tables (pipe rows) → figures (`![alt](uri)`, `<img>`) → lists (`- `, `* `, `1. `) → paragraphs (flushed on blank lines). Paragraphs are *classified* by cheap rules (`note:/warning:` → warning, `is defined as` → definition, `for example:` → example, `see ` → reference, else narrative). Citations are regex-extracted inline:

```go
citationRegex = \[(\d+|[A-Za-z]+\s+et\s+al\.,?\s*\d{4})\]
```

**Stage 2 — ChunkBuilder.** The rules that make it *semantic*:

1. **Groups.** Paragraphs and lists accumulate into a group while the section path is unchanged **and** `estimatedTokens + blockTokens <= TargetMaxTokens (700)`. Atomic blocks (table/code/equation/figure) are **never merged**.
2. **Oversize.** A single block already over `AbsoluteMaxTokens (1200)` is emitted alone, flagged `isOversized: true`, with a warning.
3. **Parent/child.** Every section that produced **more than one group** gets a synthetic parent chunk: `ParentChunkID: nil`, `ChildChunkIDs: [...]`, markdown = children joined, pages = union (sorted), offsets = min/max. Children reference the parent. A section with exactly one group → a single leaf chunk with no parent.
4. **Token estimation** is a `HeuristicSizer`: `tokens = max(charEstimate, wordEstimate)` where `charEstimate = (chars+3)/4` and `wordEstimate = words * 1.3` — deliberately conservative (overestimates), so chunks stay under budget even with dense prose.
5. **IDs** are stable and human-readable: `fmt.Sprintf("%s/%s/%03d", docID, slug, ordinal)` → `rick-rubin/tuning-in/002`. Slugs come from heading text (Unicode-normalized, lowercased, non-alphanumerics → `-`).
6. **Hashes** (the fingerprints that make differential indexing possible):

```go
ContentHash = sha256(NormalizeMarkdown(markdown))                     // content identity
Fingerprint = sha256(docID|sectionPath|headingLevel|pages|normalizedMarkdown) // structural identity
```

7. **Neighbor links.** After all chunks, `ChunkOrder = i+1` and each chunk gets `PreviousChunkID` / `NextChunkID` — so a reader can walk the book linearly even though the tree is hierarchical.
8. **Page resolution** (`resolveBlockPages`): when a PageMap exists, marker-derived page numbers are *overridden* by content matching — the block's whitespace-normalized text is searched (forward cursor, monotonic scan) in the PageMap's normalized pages; on no match, the signature is progressively shortened by quarters and rescanned. Deterministic for repeated text.

```go
// internal/pdfinspector/model/chunk.go (fields — verbatim)
type KnowledgeChunk struct {
	ChunkID          string           `json:"chunk_id"`
	ParentChunkID    *string          `json:"parent_chunk_id"`
	ChildChunkIDs    []string         `json:"child_chunk_ids"`
	PreviousChunkID  *string          `json:"previous_chunk_id,omitempty"`
	NextChunkID      *string          `json:"next_chunk_id,omitempty"`
	ChunkOrder       int              `json:"chunk_order"`
	DocumentID       string           `json:"document_id"`
	SectionPath      string           `json:"section_path"`
	HeadingLevel     int              `json:"heading_level"`
	PageNumbers      []int            `json:"page_numbers"`
	ContentMarkdown  string           `json:"content_markdown"`
	TokenEstimate    int              `json:"token_estimate"`
	CharacterCount   int              `json:"character_count"`
	Citations        []Citation       `json:"citations,omitempty"`
	SourceOffsets    SourceOffset     `json:"source_offsets"`
	ContentType      string           `json:"content_type"` // paragraph|table|code|list|equation|figure
	SemanticCategory SemanticCategory `json:"semantic_category,omitempty"`
	ContentHash      string           `json:"content_hash,omitempty"`
	Fingerprint      string           `json:"fingerprint,omitempty"`
	IsOversized      bool             `json:"is_oversized,omitempty"`
	Keywords         []Keyword        `json:"keywords,omitempty"`
	Entities         []EntityMention  `json:"entities,omitempty"`
	Concepts         []Concept        `json:"concepts,omitempty"`
	Relations        []Relation       `json:"relations,omitempty"`
	Summary          *Summary         `json:"summary,omitempty"`
}
```

## Worked Example — the *Tuning In* section, for real

From `.scratch/m5-corpus/results/rick-rubin.json`. The section produced 3 groups → 1 parent + 3 children:

```json
{
  "chunk_id": "rick-rubin/tuning-in/001",
  "parent_chunk_id": null,
  "child_chunk_ids": ["rick-rubin/tuning-in/002", "rick-rubin/tuning-in/003", "rick-rubin/tuning-in/004"],
  "previous_chunk_id": "rick-rubin/everyone-is-a-creator/001",
  "next_chunk_id": "rick-rubin/tuning-in/002",
  "chunk_order": 4,
  "document_id": "rick-rubin",
  "section_path": "Tuning In",
  "heading_level": 3,
  "page_numbers": [11, 12, 13, 14],
  "content_markdown": "Think of the universe as an eternal creative unfolding. ... ",
  "token_estimate": 1434,
  "character_count": 5737,
  "content_type": "paragraph",
  "semantic_category": "narrative",
  "is_oversized": true
}
```

And its first child:

```json
{
  "chunk_id": "rick-rubin/tuning-in/002",
  "parent_chunk_id": "rick-rubin/tuning-in/001",
  "child_chunk_ids": [],
  "previous_chunk_id": "rick-rubin/tuning-in/001",
  "next_chunk_id": "rick-rubin/tuning-in/003",
  "chunk_order": 5,
  "section_path": "Tuning In",
  "heading_level": 3,
  "page_numbers": [11, 12],
  "token_estimate": 435,
  "character_count": 1731,
  "content_hash": "4c66fd8223135a54c15b7838532b80b203f151f27144370ea28f0ba4af48795c",
  "fingerprint": "ae414c8cfedaacb32d0417e93371bba48dad73634f241d4fa7863af4c1c05ccf"
}
```

Read the data, see the design: the parent is *deliberately* `is_oversized` (1434 tokens) — it is a synthetic aggregation, never retrieved as a tight chunk; it exists so section-level questions ("what is the book's view of tuning in?") have one entry point whose children are each 400–700 tokens. The `chunk_order` chain (…001 → 002 → 003…) preserves linear reading order across the tree. The hashes are stable — re-running inspection on the same PDF yields identical hashes, which is what makes differential indexing (Part 3) skip unchanged chunks.

## Trade-offs

| Choice | Cost |
|---|---|
| 400–700 / soft 1000 / absolute 1200 | Real paragraphs longer than 700 are split mid-argument unless a boundary (blank line) is found; the parent-chunk pattern compensates but adds a synthetic level. |
| Parent chunk aggregation | Retrieval can hit a 1,400-token parent; mitigated by token budget in context assembly (Part 5) truncating oversized sources. |
| Content-based page matching | Slow on huge books (linear scan per block); mitigated by the forward cursor. If PageMap is absent, pages fall back to marker tracking — less precise. |
| Heuristic token sizing | Overestimates prose (conservative, safe); underestimates nothing — by construction. |
| Slug IDs | Readable but change if a heading changes (breaking point-ID stability — see Part 3 on why that's handled by structural point IDs, not chunk IDs). |

## Why This Matters

The chunk tree is the **atomic unit of retrieval**: every retrieved result is a `KnowledgeChunk` with section path, pages, citations, and content type. Filters (Part 4) can restrict by any of those. Parent/child links make both "give me the section" and "give me the exact paragraph" retrievable. And the `ContentHash`/`Fingerprint` pair is the foundation of differential indexing.

## What Would Break Without It

No chunk tree → no section paths → no page numbers → no citations → ARC answers become uncitable prose. No parent/child → section-level questions retrieve fragments. No stable hashes → every re-upload re-embeds the whole book (cost + latency) and the Corpus Fingerprint (Part 6) could never gate evaluation.

## Next Stop

Assets and diagnostics (Part 2.5), then the enrichment layer (Part 2.6) that derives titles, entities, keywords — then the chunk tree leaves the Go inspector as `PDFInspectionResult` and enters the **indexing half** (Part 3).

---

# Part 2.5 — Assets and Diagnostics

## Problem

Books contain things that aren't paragraphs: tables, figures, code blocks, equations, citations. Retrieval needs them as *typed, locatable* objects ("the table on page 214 with 4 columns"), not as text soup. Separately, ARC must report *how well* the pipeline ran — because a 248-page book with 3 failed pages is a different artifact than a clean one, and silent partial failures poison every downstream benchmark.

## Naive Solution

Ignore non-paragraph content ("it's all Markdown anyway") and return errors as `(result, nil)` or nothing at all. Why it fails:

- Tables-as-text lose their columns to tokenization; a "financial table" query retrieves prose fragments.
- No `skipped_pages` reporting → a book with 10 OCR-failed pages silently enters the corpus, and later retrieval benchmarks measure a *broken* book without anyone noticing.

## ARC Solution — Asset extraction

`ExtractAssetsWithContext` runs five sub-extractors in order — Table → Figure → Code → Equation → Citation — each anchored to the document with three resolvers:

- **`DefaultPageResolver`** — byte-range overlap against per-page Markdown ranges reconstructed from the PageMap → `PageContext{PrimaryPage, Pages}`.
- **`DefaultSectionResolver`** — walks the SemanticTree to find the section containing the asset; fallback to a line-by-line heading stack → `"Document Overview"`.
- **`OverlapChunkResolver`** — every chunk whose `SourceOffsets` overlap the asset's location → `RelatedChunkIDs`.

Assets are **ordered** by `SourceLocation.StartOffset` (stable sort) into `Assets.Ordered` — the document's reading flow preserved. The schema:

```go
// internal/pdfinspector/model/asset.go (abridged, verbatim)
type AssetMetadata struct {
	ID              string         `json:"id"`            // tbl-1, fig-1, code-1, eq-1, cit-1
	AssetType       AssetType      `json:"assetType"`     // table|figure|code_block|equation|citation
	PageNumber      int            `json:"pageNumber"`
	PageNumbers     []int          `json:"pageNumbers,omitempty"`
	SectionPath     string         `json:"sectionPath,omitempty"`
	SourceLocation  SourceLocation `json:"sourceLocation"` // {startOffset,endOffset,startLine,endLine}
	RelatedChunkIDs []string       `json:"relatedChunkIds,omitempty"`
}
type Assets struct {
	Tables     []Table             `json:"tables"`
	Figures    []Figure            `json:"figures"`
	CodeBlocks []CodeBlock         `json:"codeBlocks"`
	Equations  []Equation          `json:"equations"`
	Citations  []Citation          `json:"citations"`
	Warnings   []ExtractionWarning `json:"warnings,omitempty"`
	Ordered    []AssetReference    `json:"ordered,omitempty"`
	Stats      ExtractionStats     `json:"stats"`
}
```

Citation extraction runs **five passes** — bibliography items under `# References`/`# Bibliography` → `bibliography`; footnote definitions/refs (`[^x]:` / `[^x]`) → `footnote`; catalog metadata (`ISBN`, `LCCN`, `Copyright ©`) → `attribution`; inline `[N]`/`(Author et al., YYYY)` → `inline`; epigraph attributions (`Excerpt from...`) → `attribution`. All deduplicated by byte offset.

Real example (rick-rubin, abridged): the copyright block is captured as an attribution citation:

```json
{
  "id": "cit-1",
  "assetType": "citation",
  "pageNumber": 1,
  "pageNumbers": [1],
  "sectionPath": "Robert Henri",
  "sourceLocation": {"startOffset": 84, "endOffset": 559, "startLine": 3, "endLine": 3},
  "relatedChunkIds": ["rick-rubin/document-overview/001", "...", "rick-rubin/tuning-in/001"],
  "rawText": "Copyright \u00a9 2023 by Rick Rubin. Penguin Random House supports copyright. ...",
  "citationType": "attribution"
}
```

(`stats`: real output `{"tablesFound": 0, "figuresFound": 0, "codeBlocksFound": 0, "equationsFound": 0, "citationsFound": 3, "warningCount": 0, "durationMs": 58}` — a book of essays has no tables; the extractor still ran and reported honestly.)

## Diagnostics — the honesty layer

```go
// internal/pdfinspector/model/diagnostics.go (verbatim)
type Diagnostics struct {
	Status           string   `json:"status"`            // success|partial_success|failed
	ExtractionEngine string   `json:"extractionEngine"`  // "firecrawl"
	ExtractionVer    string   `json:"extractionVersion"` // "1.0.0"
	ProcessingTimeMs int64    `json:"processingTimeMs"`
	Warnings         []string `json:"warnings"`
	Errors           []string `json:"errors"`
	SkippedPages     []int    `json:"skippedPages"`
	RetryCount       int      `json:"retryCount"`
}
```

The status machine (ADR-0005):

- **`failed`** — only structural failure: encrypted PDF (`ErrEncryptedDocument`), invalid PDF (`ErrInvalidDocument`), extraction service unavailable. The inspector returns a `failed` result *alongside* the error — never nil.
- **`partial_success`** — any warning or skipped page auto-degrades from `success` (the aggregator does this; the pipeline never returns a lying `success`).
- **`success`** — everything clean.

`MapFirecrawlError` canonicalizes service error strings: `encrypted|password` → `ErrEncryptedDocument`; `invalid pdf|corrupted|malformed|not a pdf|header not found` → `ErrInvalidDocument`; everything else passes through. `SkippedPages`/`RetryCount` are extracted from the raw result's metadata.

Real diagnostics (rick-rubin — the book's only flaw is preamble text before the first heading):

```json
{
  "status": "partial_success",
  "extractionEngine": "firecrawl",
  "extractionVersion": "1.0.0",
  "processingTimeMs": 714,
  "warnings": ["preamble text detected before first heading node at page 1"],
  "errors": [],
  "skippedPages": [],
  "retryCount": 0
}
```

## Why This Matters

Assets give retrieval *typed* targets (filter by `content_type=table`). Diagnostics give every downstream benchmark a **validity gate**: the eval harness refuses to trust a book whose inspection was `failed`, and `partial_success` warnings are recorded in benchmark manifests so regressions can be traced to a specific extraction anomaly. Graceful degradation is what allows ARC to keep going on imperfect books *while never hiding the imperfection*.

## What Would Break Without It

No asset typing → table queries degrade to prose retrieval. No diagnostics → a corrupted book silently pollutes the corpus; fingerprints still match (the chunks exist), but every metric downstream is measuring garbage.

## Next Stop

The **Semantic Metadata Enrichment Layer** — the last stop inside the inspector.

---

# Part 2.6 — Semantic Metadata Enrichment

## Problem

The extraction service gives us markdown and layout, but not *knowledge*: a document's title, its language, its entities ("Rick Rubin"), its keywords, its concepts, its relations ("Rick Rubin founded Def Jam"). These enrich every chunk and the document itself — and they must be derived *non-destructively*: a failure to derive a title must never fail the pipeline.

## Naive Solution

Hardcode heuristics ("the title is the first H1") and run everything through an LLM. Why it fails: first-H1 titles break on books that begin with epigraphs or have no H1 (this corpus's real case!); LLM extraction is slow, non-deterministic, and unbounded in cost — and a determinism-breaking LLM call at ingestion time poisons the fingerprint/benchmark guarantees (Part 6).

## ARC Solution (ADR-0007) — compiler-pass architecture

```go
type EnricherPass interface {
	Name() string
	Requires() []string   // capabilities this pass needs
	Provides() []string   // capabilities this pass produces
	Execute(ctx context.Context, input *EnrichmentInput, state *PassState) error
}
```

`CompositeEnricher` runs passes in order, **skipping any pass whose requirements are unmet** (recorded in `SkippedStages`) — the compiler-pass pattern. The ten passes, in order:

1. `LanguageDetectionPass` → `language` — Turkish heuristic (chars `çğışöü` + stopwords `veya|için|bir|olarak|hakkında|bu`, ≥2 hits → `"tr"` else `"en"`). Determines downstream stopword/tokenization behavior.
2. `ChunkStatisticsPass` → fills zero `CharacterCount`/`TokenEstimate` (idempotent).
3. `TitleAuthorPass` → `resolved_title` — the **TitleResolver chain**: PDF metadata title (if non-generic) → first root heading with `PageNumbers[0] <= 5` and non-generic → cleaned filename → `"Untitled Document"`. The generic-heading denylist is real: `"extracted pdf document"`, `"chapter 1"`, `"introduction"`, `"contents"`, `"table of contents"`, `"about the author"`, `"copyright"`, ... — this is what saves ARC from naming a book after its table of contents.
4. `MetadataConsistencyPass` — PageCount from pageMap/chunks, Searchable from non-empty page markdown (only when unset).
5. `PageResolutionPass` → **`EnrichSemanticTree`** — the page-resolution workhorse. For each tree node, three strategies in order: **(A)** explicit `#`-prefixed heading line found on a page; **(B)** chunk provenance lookup (normalized `SectionPath` → chunk `PageNumbers[0]`, pre-indexed); **(C)** plain normalized text containment in non-TOC pages (TOC pages are detected by `table of contents`/dot-leader patterns and skipped). Failure → non-destructive warning: `"semantic page resolution unavailable for heading %q"`.
   - Heading normalization matters: `Beginner's Mind` ≡ `beginner’s mind` — `’`→`'`, lowercase, keep letters/digits/space, collapse whitespace.
6. `EntityExtractorPass` (hybrid: RuleBased primary + GLiNER secondary at `localhost:8088`, 3s timeout, graceful fallback) → `entities`.
7. `KeywordExtractorPass` (rule_based) → `keywords`.
8. `ConceptExtractorPass` → `concepts`.
9. `RelationExtractorPass` → `relations` (typed: `founded_by`, `located_in`, `part_of`, `relates_to`, `author_of`, `associated_with`).
10. `SummaryPass` (extractive) → `summary`.

## Worked Example — the title that isn't the title

rick-rubin's PDF metadata *has* a title ("The Creative Act: A Way of Being"), but the inspection output records... the resolved title as "Robert Henri" — the first non-generic root heading on page ≤5 — while author fell back to `"Unknown Author"`. This is the chain working as designed: metadata title existed but was rejected (or absent in the raw metadata the service returned), so the chain fell to strategy 2. If every strategy fails, the filename produces a title, and only then `"Untitled Document"`.

Real enrichment output attached to chunk `rick-rubin/tuning-in/002`:

```json
{
  "keywords": [{"value": "work", "score": 1, "source": "rule_based", "chunk_ids": ["rick-rubin/78-areas-of-thought/001", "..."]}],
  "entities": [{"text": "Rick Rubin", "type": "person", "chunk_id": "rick-rubin/tuning-in/001", "confidence": 0.9}],
  "concepts": [{"id": "concept:the-creative-act-a-way-of-being", "name": "The Creative Act: A Way of Being", "score": 0.98, "source": "rule_based"}],
  "relations": [{"id": "rel:organization:def jam recordings:founded_by:person:rick rubin",
                 "subject_id": "organization:def jam recordings", "predicate": "founded_by",
                 "object_id": "person:rick rubin", "confidence": 0.9,
                 "chunk_id": "rick-rubin/tuning-in/001", "source": "rule_based"}],
  "summary": {"text": "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons.", "source": "rule_based"}
}
```

Note the relation ID encodes the whole triple — that ID format is exactly what the M7 graph store (Part 4) persists as an entity node's evidence.

## Why This Matters

Enrichment is what makes *entity* retrieval possible (M7): the graph store is fed from `entities` + `relations`, and entity queries route to the graph fusion path. Keywords and language configure sparse retrieval (Part 4). The pass architecture means future enrichments (e.g. an LLM summarizer) are additive and individually gated — they can never regress the rest of the pipeline.

## What Would Break Without It

No entities/relations → the M7 entity graph is empty → entity queries degrade to plain dense retrieval (the exact regression the M7 benchmark measured and the `UseGraph` gate guards). No page resolution → headings lose page numbers → the Semantic Metadata Enrichment promise dies and citations become vaguer. No language → Turkish stopword handling breaks sparse retrieval.

## Next Stop

The inspector emits the complete `PDFInspectionResult` — `{document, semanticTree, content, chunks, assets, diagnostics}` — and hands it to the **Indexing Worker** (Part 3): diff, embed, upsert.

---

# Part 3 — Indexing: Embeddings, Differential Diff, Qdrant

## Problem

196 chunks of Markdown must become *searchable by meaning*. Meaning is represented as dense vectors (embeddings) — and, in ARC, also as sparse BM25 vectors. The vectors must live in a persistent store (Qdrant) that supports both kinds of search plus metadata filtering. And indexing must be **incremental and idempotent**: re-uploading a fixed book must not re-embed 196 chunks; a changed paragraph must re-embed only what changed; a removed section must be deleted.

Two sub-problems hide here:

1. **Identity**: how do we know point `X` is "the same chunk" across runs? (A chunk whose *heading slug* changed is arguably a different chunk.) What is the identity key?
2. **Change detection**: how do we know a chunk's *content* changed without comparing megabytes of text? (A content hash — we already have one from Part 2.)

## Naive Solution

On every upload: embed everything, wipe the document's points, re-insert. Why it fails: re-embedding 196 chunks costs ~196 API calls and seconds-to-minutes per book on every upload — even when nothing changed; and wipe-then-reinsert makes a mid-crash state where the document has *no* points (readers see a book that vanished).

## ARC Solution (ADR-0008, ADR-0026, ADR-0028)

### The embedding seam

```go
// internal/indexing/provider/provider.go (verbatim)
type EmbeddingProvider interface {
	EmbedDocuments(ctx context.Context, texts []string) (*EmbeddingResult, error) // batch
	EmbedQuery(ctx context.Context, query string) ([]float32, error)              // single
	Capabilities() ProviderCapabilities  // Dimension, MaxBatchSize, MaxInputTokens, SupportsBatch
	Provider() string                    // "Ollama"
	Model() string                       // "nomic-embed-text:latest"
	Health(ctx context.Context) error
}
```

Only two concrete providers exist — this is a deliberate ADR-0026 outcome:

- **`OllamaEmbeddingProvider`** — the local backend: `nomic-embed-text`, dimension **768**, `MaxBatchSize: 100`, `MaxInputTokens: 8192`. Nomic's retrieval prefixes live inside the adapter (`"search_document: "` for documents, `"search_query: "` for queries) — a domain detail hidden from the seam. The embedding *version* is pinned to `"1.0.0"` and the model tag must be pinned (never `:latest`) — because the version string is part of the index signature (below), and a drifting model would silently invalidate every signature.
- **`MockEmbeddingProvider`** — deterministic, thread-safe, offline: SHA-256-seeded pseudo-vectors, L2-normalized. This is how tests and CI run the *entire* pipeline without any network.

There is no OpenAI/Gemini/Voyage provider despite the ADR-0008 naming them — ADR-0026 deliberately keeps embeddings local (Ollama) and only *generation* behind the OpenAI-compatible gateway. The fixture values `"OpenAI"/"text-embedding-3-large"` you'll find in tests are legacy literals, not shipped providers — a known inconsistency worth remembering when reading tests.

### The index signature — how "same chunk" is decided

```go
// internal/indexing/model/signature.go
func CalculateIndexSignature(contentHash, provider, modelName, version, schemaVer string) string {
	raw := fmt.Sprintf("%s:%s:%s:%s:%s", contentHash, provider, modelName, version, schemaVer)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}
```

The signature digest wraps **five** inputs: `ContentHash:EmbeddingProvider:EmbeddingModel:EmbeddingVersion:ChunkSchemaVer`. Change any one — the chunk text, the provider, the model, the embedding version, the chunk schema — and the signature changes, forcing re-embedding. This is the *differential* mechanism: a signature is cheap to compare (64 hex chars) and encodes *why* a chunk is stale.

### The point ID — structural identity

```go
// internal/indexing/store/point.go
func CalculatePointID(documentID, sectionPath string, chunkOrder int) string {
	raw := fmt.Sprintf("%s:%s:%d", documentID, sectionPath, chunkOrder)
	return graphmodel.CalculatePointID(raw) // SHA-256, RFC-4122 UUID formatting
}
```

Identity is **structural location**, not the chunk ID: `SHA256(DocumentID:SectionPath:ChunkOrder)`. Why? Because the chunk ID embeds the heading slug (`tuning-in`), which changes when a heading changes — but a chunk that moved sections *is* a different chunk anyway (its section path changed), so it should be delete+new, not modified. And `ChunkOrder` disambiguates two chunks in the same section.

### The diff engine

`internal/indexing/diff/engine.go` — pure function, fully unit-tested:

```go
func (e *Engine) ComputeDiffPlan(documentID string, chunks []KnowledgeChunk, existing []VectorMetadata) *DiffPlan {
	// 1. Index existing points by point ID.
	// 2. For each incoming chunk: same point ID?
	//      no  -> NewChunks
	//      yes -> IndexSignature equal? UnchangedChunks : ModifiedChunks
	// 3. Any existing point for this document not matched -> DeletedPointIDs + DeletedChunkIDs.
}
```

The plan is then executed by the worker (`internal/indexing/worker/worker.go`, `ExecuteSync` — the only execution path today; the async queue is planned, not built):

```
Pending → Running
 1. ListPoints(filter: document)                    // read existing
 2. ComputeDiffPlan                                 // pure
 3. Delete removed points + their content           // delete BEFORE embed
 4. (M7) write entity graph nodes                   // before vector upsert, so a graph
                                                    // failure fails the job with nothing persisted
 5. Embed only ChunksToEmbed(), batched by MaxBatchSize
 6. Build VectorMetadata + point payload
 7. (opt-in) encode sparse BM25 vectors
 8. UpsertPoints + PutContent
    → Completed (job stats: indexed / skipped / deleted)
```

The `IndexingJob` state machine (`Pending → Running → Completed | Failed | Retrying | Cancelled`) is the ADR-0008/0014 pattern; today only `Pending → Running → Completed/Failed` is exercised.

## Internal Data — the payload envelope

Every point carries the full provenance envelope — the bridge between ingestion and retrieval:

```go
// internal/indexing/model/metadata.go (verbatim)
type VectorMetadata struct {
	WorkspaceID       string   `json:"workspace_id,omitempty"`
	KnowledgeSpaceID  string   `json:"knowledge_space_id,omitempty"`
	DocumentID        string   `json:"document_id"`
	ChunkID           string   `json:"chunk_id"`
	ChunkOrder        int      `json:"chunk_order"`
	SectionPath       string   `json:"section_path,omitempty"`
	PageNumbers       []int    `json:"page_numbers,omitempty"`
	ContentType       string   `json:"content_type,omitempty"`
	Citations         []string `json:"citations,omitempty"`
	ContentHash       string   `json:"content_hash,omitempty"`
	EmbeddingProvider string   `json:"embedding_provider,omitempty"`
	EmbeddingModel    string   `json:"embedding_model,omitempty"`
	EmbeddingVersion  string   `json:"embedding_version,omitempty"`
	ChunkSchemaVer    string   `json:"chunk_schema_version,omitempty"`
	IndexSignature    string   `json:"index_signature,omitempty"`
}
```

This is exactly what lands in the Qdrant payload (keys: `workspace_id, knowledge_space_id, document_id, chunk_id, chunk_order, section_path, content_type, content_hash, embedding_provider, embedding_model, embedding_version, chunk_schema_version, index_signature` + `page_numbers` (list, only if non-empty) + `citations` (list, only if non-empty) + `content_markdown` (added at upsert, only if non-empty)). The comment in the code is a rule: *"Keep this schema stable: it is the source of truth for filter translation and metadata reconstruction."*

## Qdrant specifics (ADR-0028)

- **One collection** (`arca_chunks`) holds dense **and** sparse: dense under the empty named vector `""`, sparse under the named vector `"sparse"`. Retrieval state is never process-local — sparse vectors are persisted, restored by `ListPoints`, and searched via `QueryPoints` with `Using: "sparse"`.
- **Cosine distance**, dimension from provider capabilities (768 for nomic).
- Sparse is opt-in (`SPARSE_INDEX=true`) because enabling it requires *recreating the collection* (Qdrant collection config is immutable) — a migration, not a toggle.
- gRPC on port 6334, `SkipCompatibilityCheck: true`; scroll API (batch 100) for `ListPoints` (restores vectors too — needed for diffing), payload round-trip via `metadataToPayload`/`payloadToMetadata`.
- The `ContentStore` (`InMemoryContentStore`) is a *separate, process-local* seam storing `ChunkID → ContentMarkdown` — the fallback content source; the durable source is the Qdrant payload. (This split caused the M7 BULGU-1 bug: graph retrieval read content from the process-local store, which is empty in a fresh process. The fix: payload-first, ContentStore fallback.)

### BM25 sparse encoding (M3, ADR-0028)

```go
// internal/indexing/sparse/encoder.go
type SparseVector struct { Indices []uint32; Values []float32 }
type SparseEncoder interface { Encode(ctx context.Context, text string) (SparseVector, error) }
```

`BM25Encoder` with k1=1.5, b=0.75, lowercase alphanumeric tokens, **corpus-wide IDF computed at index time** — and the deliberate, documented trade-off: IDF is *stale* for incrementally grown corpora (recomputation is an M4 concern; contributors must not "fix" it ad hoc). Term IDs are assigned from a **sorted vocabulary**, so indices are deterministic. Qdrant sparse scoring is a pure dot product.

## Worked Example — re-uploading a fixed book

1. First upload of rick-rubin: `ListPoints(document=rick-rubin)` → 0 points. Diff: all 196 chunks `NewChunks`. Embed 196 (batch of 100 + 96), upsert 196 points. Job: `indexed=196, skipped=0, deleted=0`.
2. Fix a typo in the book and re-upload: `ListPoints` → 196 existing. Diff: 195 chunks have identical `ContentHash` → identical `IndexSignature` → `UnchangedChunks` (skipped, **zero embedding calls**); 1 chunk (`tuning-in/002`, say) has a new hash → `ModifiedChunks` → 1 embed + 1 upsert. Job: `indexed=1, skipped=195, deleted=0`.
3. Remove a section: its point ID disappears from the incoming set → `DeletedPointIDs`; points and their content-store entries are deleted *before* embedding, so a crash mid-run never leaves a partial re-embedding of a deleted section.

Real point fixture (tests, `qdrant_test.go`) — the shape after upsert:

```go
{ ID: "pt-1", Vector: []float32{1,0,0}, ContentMarkdown: "Sample markdown...",
  Metadata: model.VectorMetadata{
	DocumentID: "doc-1", ChunkID: "chk-1", ChunkOrder: 0, SectionPath: "Intro",
	PageNumbers: []int{1,2}, ContentType: "paragraph", Citations: []string{"[1] Smith et al."},
	ContentHash: "hash-1", EmbeddingProvider: "Ollama", EmbeddingModel: "nomic-embed-text",
	EmbeddingVersion: "1.0.0", ChunkSchemaVer: "1.0", IndexSignature: "sig-1",
  } }
```

## Trade-offs

| Choice | Cost |
|---|---|
| Signature = digest of 5 fields | A provider/model/version/schema bump invalidates *every* point (full re-index) — correct but expensive; hence the "pin your model tag" rule. |
| Structural point IDs | A chunk that moves sections becomes delete+new (loses its embedding cost once); correct semantics, extra ops. |
| Corpus-wide IDF at index time | Stale IDF for grown corpora (documented, accepted). |
| Sparse in same collection | Collection recreation to enable; but zero extra persistence layer. |
| ContentStore as separate seam | Process-local memory — the BULGU-1 footgun; mitigated by payload-first reads. |
| Sync-only worker | No queue yet — `ExecuteSync` is called directly; the async job/QAJob states exist but only the sync path is live. |

## Why This Matters

Differential indexing is what makes **re-inspection cheap and safe**: `arc inspect` on a fixed PDF is a ~1-embedding operation, not a full re-embed. The signature is also what makes the **Corpus Fingerprint** (Part 6) meaningful — the fingerprint is computed over the *persisted* `ContentHash` values, i.e. over what the index actually contains. And the payload envelope is what makes metadata filters and citations work on the other side.

## What Would Break Without It

Without diffing: every upload re-embeds everything (cost ×196 for rick-rubin), and mid-crash states lose whole documents. Without the signature: a model upgrade silently mixes old and new embeddings — retrieval quality degrades with no signal. Without payload metadata: no filters, no citations, no per-document benchmarks.

## Next Stop

The index is populated. **Part 4** is the read side: dense, sparse, hybrid RRF, and the graph fusion path — and the orchestration that picks among them.

---

# Part 4 — Retrieval: Seam, Dense, Sparse, Hybrid RRF, Graph

## Problem

Given a question ("What does the book say about Rick Rubin?"), return the 5 most relevant chunks — with a score, their metadata, and their content. The answer must be *deterministic* (same query → same ranking), *filterable* (by document, page, content type), *thresholdable* (abstain when nothing clears the bar), and *composable* (multiple retrieval strategies that can be fused or swapped behind one interface).

## Naive Solution

One function that queries Qdrant with the query embedding and returns top-k. Why it fails:

- **One strategy misses.** Lexical matches (exact words, proper nouns) and semantic matches (paraphrase) find different things; a single dense-only search misses "BM25" and vice versa.
- **Scores lie across strategies.** Dense cosine scores and sparse BM25 scores live on different scales; you can't merge them by summing raw scores — the dense stream would always win.
- **No separation of concerns.** Retrieval internals leak into the QA layer; benchmarks can't isolate "which strategy helped".
- **No filters.** "Give me the page-12 tables of *Asking the Right Questions*" is an impossible query.

## ARC Solution

### The seam (everything behind one interface)

```go
// internal/retrieval/seam/retriever.go + query.go + result.go (verbatim)
type Retriever interface {
	Retrieve(ctx context.Context, query RetrievalQuery) ([]SearchResult, error)
}

type RetrievalQuery struct {
	QueryText string                       `json:"query_text"`
	TopK      int                          `json:"top_k"`
	Mode      RetrievalMode                `json:"mode"` // Dense|Sparse|Hybrid
	Filter    indexingmodel.MetadataFilter `json:"filter,omitempty"`
	MinScore  float32                      `json:"min_score,omitempty"`
	Stats     *RetrievalStats              `json:"-"` // diagnostics, nil = no overhead
}

type SearchResult struct {
	ChunkID         string                       `json:"chunk_id"`
	ContentMarkdown string                       `json:"content_markdown,omitempty"`
	Score           float32                      `json:"score"`
	Metadata        indexingmodel.VectorMetadata `json:"metadata"`
}
```

Three invariants this seam enforces:

- **Deterministic total order** — `SortResultsByScore`: score desc, then ChunkID asc. Map-iteration randomness must never reach a caller.
- **Score threshold lives at the store** for dense/sparse (Qdrant `ScoreThreshold`), *inside the retriever* for graph (its scores are its own scale). Never post-fusion.
- **Content is payload-first, ContentStore fallback** — the BULGU-1 fix (dense, sparse, and graph all share this rule).

### Dense retriever

`EmbedQuery` (query-side prefix `search_query:`) → `SearchVector{Vector, TopK, Filter, MinScore}` → Qdrant `SearchPoints` with `ScoreThreshold` → map to `SearchResult`, fill content, sort. ~60 lines. Boring on purpose.

### Sparse retriever

Mirror of dense, `Mode=Sparse`: `encoder.Encode(query)` (corpus IDF from `EncoderForCorpus`) → `SearchVector{Sparse, ...}` → Qdrant `QueryPoints` with `Using: "sparse"`. **Empty encoding → empty results, no store call** (a query with no corpus terms cannot match).

### Hybrid retriever + RRF (M3/M4)

The fusion core, verbatim — this function is the heart of hybrid retrieval:

```go
// internal/retrieval/hybrid/fusion.go
// score(d) = sum over streams m of weight_m / (k + rank_m(d))
func ReciprocalRankFusion(streams [][]seam.SearchResult, k float64, weights ...float64) []seam.SearchResult {
	if k <= 0 { k = 60.0 }
	scoreMap := make(map[string]float64)
	resultMap := make(map[string]seam.SearchResult)
	for si, stream := range streams {
		w := 1.0
		if si < len(weights) && weights[si] > 0 { w = weights[si] }
		for rank, res := range stream {
			rrfScore := w / (k + float64(rank+1))
			scoreMap[res.ChunkID] += rrfScore
			if existing, exists := resultMap[res.ChunkID]; !exists || res.ContentMarkdown != "" {
				resultMap[res.ChunkID] = res
			} else { _ = existing }
		}
	}
	fused := make([]seam.SearchResult, 0, len(resultMap))
	for chunkID, res := range resultMap { res.Score = float32(scoreMap[chunkID]); fused = append(fused, res) }
	seam.SortResultsByScore(fused)
	return fused
}
```

Why RRF works where raw-score fusion fails: a chunk ranked #1 in the dense stream and #9 in the sparse stream gets `1/61 + 1/69 ≈ 0.0308`, and a chunk ranked #2 in *both* gets `2×(1/62) ≈ 0.0323` — **rank agreement beats scale agreement**. Cosine 0.9 (dense) and BM25 12.4 (sparse) never needed to be comparable.

The weights are not tunable at runtime — they are the **frozen, benchmark-calibrated `FusionPolicy`** (M4):

```go
type FusionPolicy struct { DenseWeight, SparseWeight float64; SparseCap int; RRFK float64 }
// Balanced (production):     Dense 1.0, Sparse 1.0, cap 0, k 60   (ADR-0036 — the M3-compatible default)
// DenseBiased (sweep result): Dense 1.0, Sparse 0.5, cap 0, k 60   (recovers comparison/procedural regressions)
// LexicalBiased: NOT DEFINED — errors. A sparse-biased policy needs benchmark evidence to exist.
```

The hybrid call sequence: validate+normalize → **detached sub-calls** (sub-retrievers get `Stats=nil`; the hybrid counts stream lengths itself — sub-retrievers must never write into the shared aggregate) → SparseCap truncation on the sparse stream *before* fusion → RRF with policy weights → truncate to TopK. MinScore is forwarded per-stream (threshold at the store), **never applied post-fusion** — fusion only reorders, it never filters. Empty streams → empty result (abstention signal upstream).

## Worked Example — real benchmark data

From `docs/benchmarks/baseline_dense_v1.json` (real corpus, rick-rubin, TopK 5): query `rr-001` (single_fact) retrieved:

```json
{
  "retrieved_chunk_ids": ["rick-rubin/point-of-view/002", "rick-rubin/point-of-view/001",
                          "rick-rubin/the-unseen/001", "rick-rubin/translation/001",
                          "rick-rubin/translation/002"],
  "retrieved_scores": [0.8656716, 0.8358971, 0.78755856, 0.7442996, 0.74350697],
  "expected_chunk_ids": ["rick-rubin/point-of-view/001", "rick-rubin/point-of-view/002"],
  "mrr": 1, "ndcg_at_k": 1, "recall_at_k": 1, "precision_at_k": 0.4,
  "stats": {"duration_ms": 56, "candidates": 5, "top_k_requested": 5, "top_k_returned": 5, "min_score": 0}
}
```

Rank 2 is relevant (0.836); the expected chunks are both within top-2 → MRR 1.0 (first relevant at rank 1), nDCG@5 1.0 (both relevant chunks are the top-2), precision 2/5. That's what "recall 1.0, nDCG 1.0" means on this query — and why the benchmark gates measure rank quality, not raw scores.

## The M7 graph path (ADR-0038..0042)

Entity queries get a third stream. The story, compressed:

1. **Persistence** (ADR-0038): the graph store (`arca_graph_nodes`) is *entity-only* — nodes, no edges (v1). Qdrant, **vectorless points**, REST (gRPC rejects vectorless upserts). Chunk evidence is **one payload field per chunk** (`chunk_<hex>`, hex-encoded because `/` and `.` break Qdrant payload JSON paths), written via `set_payload` — concurrent writers can't clobber each other (M7 audit A1: the old read-modify-write lost 19/20 updates under concurrency). Scores recorded per document as `score_<hex>`, reads take the max.
2. **Retrieval** (ADR-0039): `GraphRetriever` — no query-side NER (accepted trade-off). Query tokens (lowercased, stopworded — the same stopword set as the kill-gate prototype) match entity *names* by substring; score = `Σ entity.Score × token_coverage` (capped 1.0), per-chunk evidence summed across matching entities; MinScore applied *retriever-side*; content payload-first (BULGU-1). Deterministic: score desc, ChunkID asc.
3. **Fusion** (ADR-0041): `GraphFusionRetriever{dense, graph, config{DenseWeight: 1.0, GraphWeight: w}}` — RRF over two streams with the *frozen* k=60; `GraphWeight <= 0` → dense stream returned **byte-identical** (the rejection path costs nothing). Calibration swept w ∈ {0.5, 1.0, 2.0} and froze **1.0** (Gold Set v3: entity recall 0.840→, outside recall 1.000, nDCG@5 0.879 — no regression beyond the 5% tolerance).
4. **Gate** (ADR-0042): `UseGraph = (intent == entity) ∧ (graphWeight > 0)` — decided once in the orchestrator; the engine swaps the execution component; eval flags (`--graph-weight`, `--graph-only`) never share a decision mechanism with production config (`RETRIEVAL_GRAPH_WEIGHT`).

## MetadataFilter — the domain-filter bridge

```go
// internal/indexing/model/filter.go (verbatim)
type MetadataFilter struct {
	WorkspaceID       string     `json:"workspace_id,omitempty"`
	KnowledgeSpaceID  string     `json:"knowledge_space_id,omitempty"`
	DocumentIDs       []string   `json:"document_ids,omitempty"`
	ChunkIDs          []string   `json:"chunk_ids,omitempty"`
	PointIDs          []string   `json:"point_ids,omitempty"`
	PageNumbers       []int      `json:"page_numbers,omitempty"`
	ContentTypes      []string   `json:"content_types,omitempty"`
	SectionPathPrefix string     `json:"section_path_prefix,omitempty"`
	IndexedAfter      *time.Time `json:"indexed_after,omitempty"`
}
```

Strongly typed, database-agnostic: it rides on `RetrievalQuery.Filter` and is translated to Qdrant conditions (`document_id` → keywords-in, `page_numbers` → match ints, `content_type` → keywords-in). Two caveats documented in code: `SectionPathPrefix` and `IndexedAfter` are honored by the in-memory store but **not translated to Qdrant** (unimplemented there). The domain never speaks Qdrant's query language.

## Why This Matters

The retriever seam + frozen fusion config is what makes **benchmark-gated evolution** possible: every new strategy (sparse in M3, graph in M7, rerank in M8) lands as a composition behind the same interface, measured against the same Gold Set, and shipped only when it beats the frozen baseline. The `RetrievalStats` envelope makes every benchmark run self-describing (candidates, thresholds, fused counts).

## What Would Break Without It

No seam → QA couples to Qdrant and no strategy can be measured in isolation. No RRF → dense and sparse scores can't be combined truthfully. No frozen policies → runtime tuning destroys benchmark comparability. No graph gate → entity queries either miss the graph entirely or pay graph costs on every query. No filters → the KnowledgeSpace isolation story (workspace/space/document) has no enforcement point at retrieval.

## Next Stop

**Part 5** — the AnswerEngine: how a query becomes an analyzed, orchestrated, gated, generated, verified `Answer`.

---

# Part 5 — The AnswerEngine: Analysis, Orchestration, Gate, Generation, Verification

## Problem

A user asks a natural-language question. ARC must decide *how to search* (decompose? graph? plain?), retrieve, assemble a token-budgeted context, **decide whether the evidence actually answers the question** (abstain if not — do not hallucinate), generate a grounded answer with `[Ref N]` citations, and verify that every reference the LLM emitted actually exists in the retrieved sources.

The hardest constraint: **abstention is a feature**. An answer to a question the corpus cannot answer is worse than no answer — it is a lie with citations.

## Naive Solution

Embed the query, retrieve top-5, paste into a prompt, print. Why it fails: comparison questions retrieve a mix of both sides' chunks and the LLM blends them vaguely; entity questions miss the graph; context windows overflow with duplicate chunks; and when the corpus *can't* answer, the LLM confidently invents an answer anyway (the canonical RAG failure).

## ARC Solution — the full `Answer()` flow

```go
// internal/qa/engine.go (control flow, condensed)
func (e *AnswerEngine) Answer(ctx context.Context, query seam.RetrievalQuery) (*Answer, error) {
	// 1. Analyze — deterministic, rule-based (M5)
	analyzed := e.analyzer.Analyze(query.QueryText)
	// 2. Orchestrate — IntentHint + RuntimeConfig → RetrievalDecision (M6/M7)
	hint := AnalyzeIntentHint(analyzed)
	decision := DecideRetrievalRouting(hint, e.retrievalCfg)
	// 3. Execute — swap component if UseGraph, decompose if comparison
	exec := e.retriever
	if decision.UseGraph && e.graphRetriever != nil { exec = e.graphRetriever }
	if decision.Decompose {
		// per sub-query: Retrieve(sub, TopK=budget); MergeRankedLists(lists, budget)
	} else {
		draft.SearchResults = exec.Retrieve(query)
	}
	// 4. Abstain on empty retrieval — no gate call, no LLM call
	if len(draft.SearchResults) == 0 { return noEvidenceAnswer() }
	// 5. Context assembly — dedup, budget, [Ref N]
	win := e.contextBuilder.Build(draft.SearchResults)
	// 6. Evidence gate — the pre-generation semantic abstention (ADR-0033/0034)
	decision := e.evaluateGate(ctx, query.QueryText, win)   // 1 retry on gate_error
	if decision == EvidenceUnsupported { return noEvidenceAnswer() }
	// 7. Prompt → LLM → verify (Phase 1 structural)
	prompt := e.promptBuilder.Build(query.QueryText, win)
	llmResp := e.llmProvider.Generate(prompt)
	verified := e.verifier.Verify(llmResp.Content, win)
	return &Answer{Text, Citations, Verification, Status, Metadata}, nil
}
```

### 1. The rule-based analyzer (M5) and its 6 regexes

```go
type AnalyzedQuery struct {
	RawQuery        string                       `json:"raw_query"`
	Intent          string                       `json:"intent"`      // concept_lookup | entity_lookup | procedural_lookup
	Entities        []string                     `json:"entities,omitempty"`
	ExtractedFilter indexingmodel.MetadataFilter `json:"extracted_filter,omitempty"` // never populated by the rule-based analyzer
	SubQueries      []string                     `json:"sub_queries,omitempty"`
}
```

Intent rules: starts with `who` or contains `author` → entity; matches `^what does the (?:book|it) say about .+\??$` → entity; starts with `how` → procedural; else concept. Entities = whitespace fields longer than 3 chars, punctuation trimmed — deliberately naive (no NER).

**Decomposition** — 6 regexes, `nil` if no match; sides split and stripped of empties:

```
^compare\s+(.+?)\s+(?:with|and)\s+(.+)$
^(.+?)\s+vs\.?\s+(.+)$
^.*?\bdifference\s+between\s+(.+?)\s+and\s+(.+)$
^how\s+do\s+(.+?)\s+and\s+(.+?)\s+differ$
^contrast\s+(?:the\s+approaches\s+of\s+)?(.+?)\s+and\s+(.+)$
^what\s+distinguishes\s+(.+?)\s+from\s+(.+)$
```

### 2. The orchestrator — minimal and benchmark-gated (M6/M7)

```go
// internal/qa/orchestration.go (verbatim)
func DecideRetrievalRouting(hint IntentHint, cfg RetrievalRuntimeConfig) RetrievalDecision {
	switch hint.Intent {
	case HintIntentComparison:
		decision := RetrievalDecision{Decompose: true}
		if cfg.ComparisonTopK > 0 { decision.TopKOverride = cfg.ComparisonTopK }
		return decision
	case HintIntentEntity:
		if cfg.GraphWeight > 0 { return RetrievalDecision{UseGraph: true} }
	}
	return RetrievalDecision{}
}
```

`RetrievalDecision{Decompose bool, TopKOverride int, UseGraph bool}` — nothing else. No `Policy` field: runtime fusion selection is *config-frozen* (`Balanced`, ADR-0036); `TopKOverride` (evidence budget, calibrated **8** on Gold Set v2, ADR-0037) applies to comparison sub-queries; `UseGraph` opens only for entity intent with `GraphWeight > 0` (ADR-0042). The engine decides once; retrievers execute. Comparison sub-results merge via `seam.MergeRankedLists` — round-robin interleave by rank, dedup by ChunkID, truncate to budget.

### 4 & 6. Abstention — two doors, one answer

Both abstention paths return the same shape — an answer that is *honest* about having no evidence:

```go
&Answer{Text: "The retrieved sources do not cover this query, so no grounded answer can be provided.",
         Status: qaverification.StatusNoEvidence}
```

Door 1: **empty retrieval** (nothing cleared `MinScore`; no gate call, no LLM call). Door 2: the **EvidenceGate** (ADR-0033/0034):

```go
type EvidenceDecision string // "supported" | "unsupported" | "gate_error"
type EvidenceGate interface {
	Evaluate(ctx context.Context, query string, win *qacontext.ContextWindow) (EvidenceDecision, error)
}
```

The `LLMEvidenceGate` asks the LLM one question — "do these sources answer this question?" — temperature 0, `MaxTokens: 512`, strict single-JSON parse (`{"decision": "supported"|"unsupported"}`). Malformed output → `gate_error` → **one retry** (`MaxGateAttempts = 2`) → then a typed `EvidenceGateError` (fail-closed: an operational failure must never be read as "no evidence").

### 5. Context assembly — the `[Ref N]` contract

```go
type SourceReference struct {
	CitationKey string                       `json:"citation_key"` // "[Ref 1]"
	DocumentID  string                       `json:"document_id"`
	ChunkID     string                       `json:"chunk_id"`
	SectionPath string                       `json:"section_path,omitempty"`
	PageNumbers []int                        `json:"page_numbers,omitempty"`
	Content     string                       `json:"content"`
	Metadata    indexingmodel.VectorMetadata `json:"metadata"`
}
type ContextWindow struct {
	Sources    []SourceReference `json:"sources"`
	Content    string            `json:"content"`
	TokenCount int               `json:"token_count"`
}
```

`DefaultContextBuilder.Build`: dedup by ChunkID (keep first — rank order preserved), assign `[Ref 1]..[Ref N]` sequentially, block format `%s Source: %s (Page %v, Section: %s)\n%s` — then **token budget** (default 4000, `SimpleTokenCounter` ≈ max(words, len/4)): the first source always admits, then accumulation stops. This is the *immutable citation contract*: the keys emitted here are exactly what verification checks later.

### 7. Generation and verification

`RAGPromptBuilder` produces the system instruction ("…Always cite your claims using inline reference markers like [Ref 1], [Ref 2]… Never invent or hallucinate…") + user `"CONTEXT:\n%s\n\nUSER QUESTION:\n%s"` with `Temperature: 0.2, MaxTokens: 1500`. The `LLMProvider` seam (`Generate`/`Stream`/`Capabilities`) has exactly one real adapter: the **OpenAI-compatible gateway** (ADR-0026 — `LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL`; `LLM_PROVIDER` is an observability-only label).

Then `VerificationPipeline.Verify`:

- **Phase 1 (structural, implemented)**: `CitationExtractor` expands `[Ref 1, 2]` → `[Ref 1] [Ref 2]`, collects every `[Ref N]` marker; each must resolve against the ContextWindow's `SourceReference`s. `TotalClaims` = markers; missing keys → `InvalidReferences`; found → `VerifiedClaims`. Status = `verified` iff `InvalidReferences == 0 && VerifiedClaims > 0`, else `unverified`.
- **Phase 2 (NLI entailment)**: the seam exists (`EntailmentChecker` with `entailed|contradicted|neutral`) but **no implementation and never invoked** — a documented gap, not a secret.
- `no_evidence` is produced only by the engine's abstention paths, never by the pipeline.

The final `Answer` is provider-neutral:

```go
type Answer struct {
	Text         string                        `json:"text"`
	Citations    []qacitation.AnswerCitation   `json:"citations"`    // {CitationKey, DocumentID, ChunkID, SectionPath, PageNumbers, Snippet}
	Verification qacitation.VerificationReport `json:"verification"` // {TotalClaims, VerifiedClaims, MissingCitations, InvalidReferences}
	Status       qaverification.VerificationStatus `json:"status"`   // verified | unverified | no_evidence
	Metadata     AnswerMetadata                `json:"metadata"`     // {Provider, Model, Usage} — never provider-specific fields
}
```

## Worked Example — a comparison question, end to end

Query: *"Compare the approaches of Donella Meadows and Michael Stack."*

1. **Analyze**: intent = concept; `decomposeComparison` matches `^contrast\s+(?:the\s+approaches\s+of\s+)?(.+?)\s+and\s+(.+)$` → `SubQueries = ["Donella Meadows", "Michael Stack"]`.
2. **Hint**: non-empty SubQueries → `HintIntentComparison`. **Decision**: `{Decompose: true, TopKOverride: 8}`.
3. **Execute**: for each sub-query, retrieve TopK=8 (evidence budget); each stream is scored *in its own context* ("Donella Meadows" alone, "Michael Stack" alone — retrieval quality per side instead of a blended single search); `MergeRankedLists` interleaves: rank-1 from side A, rank-1 from side B, rank-2 from A, ... deduped, truncated to 8.
4. **Gate**: "do these 8 chunks answer 'compare the approaches of...'?" — if the corpus only covers one side, `unsupported` → `no_evidence` (honest abstention instead of a one-sided comparison).
5. **Generate**: context with `[Ref 1]`…`[Ref 8]`; the LLM cites per side. **Verify**: every `[Ref N]` must exist; count claims; status.

## Why This Matters

The engine is where *safety* lives: abstention doors (empty retrieval, gate), citation contract (immutable keys), fail-closed operational errors, provider-neutral outputs. Every M-milestone is a *contract change* here, not a rewrite — M5 added the gate, M6 added orchestration, M7 added the graph slot, M8 (probe pending) adds reranking behind the same retriever seam.

## What Would Break Without It

No analyzer → comparison questions degrade to blended retrieval (the M6 regression the benchmark caught). No gate → hallucinated answers with fake confidence. No citation contract → verification is impossible and answers unciteable. No provider-neutral `Answer` → swapping LLM providers leaks into the domain model.

## Next Stop

**Part 6** — the evaluation framework that decides whether any of this is *true*: Gold Sets, fingerprints, metrics, gates — and the agent layer that sits on top.

---

# Part 6 — Evaluation: Gold Sets, Fingerprints, Metrics, Gates

## Problem

Every subsystem above made a bet: "this strategy is better than that one." Without measurement, those bets are opinions. ARC's discipline (ADR-0027) is that **any change to retrieval behavior must prove itself against a fixed, versioned benchmark before it reaches production** — and that a rejected change must be trivially reversible.

## Naive Solution

Run 10 hand-picked questions against old vs new, eyeball the answers, ship the better-looking one. Why it fails: hand-picked questions overfit; eyeballing is irreproducible; "better-looking" is not a number; and there is no defense against silently testing on the same queries you tuned on.

## ARC Solution

### The Gold Set (ADR-0027) — versioned, human-curated, corpus-grounded

```go
// internal/eval/goldset.go (verbatim)
type GoldSet struct {
	SchemaVersion string       `json:"schema_version"`
	Corpus        CorpusInfo   `json:"corpus,omitempty"`
	Documents     []CorpusInfo `json:"documents,omitempty"` // schema 1.2: multi-document
	Queries       []GoldQuery  `json:"queries"`
}
type GoldQuery struct {
	ID                 string   `json:"id"`
	Intent             string   `json:"intent"`    // single_fact|concept|procedural|comparison|entity|abstention
	Query              string   `json:"query"`
	ExpectedChunkIDs   []string `json:"expected_chunk_ids"`
	ExpectedSections   []string `json:"expected_sections"`
	ExpectedNoEvidence bool     `json:"expected_no_evidence"`
}
```

Rules that make it trustworthy: queries are built **exclusively from the real indexed corpus** (no synthetic facts); abstention queries declare `expected_no_evidence: true` (measuring whether the pipeline *abstains* instead of forcing nearest-neighbor answers); validation rejects any query that declares expected chunks while `expected_no_evidence` (and vice versa). Versioned: v1, v1.1, v2, v3 — each a new file, never mutated in place.

Real queries from `goldset_v3.json` (37 queries, 5 books):

```json
{"id": "g-ent-01", "intent": "entity",
 "query": "What does the book say about Bowling Green State University?",
 "expected_chunk_ids": ["Browne.../asking-the-right-questions-m-neil-browne-stuart-m-keeley-bowling-green-state-university/001",
                        "Browne.../asking-the-right-questions-m-neil-browne-stuart-m-keeley-bowling-green-state-university/003"],
 "expected_sections": ["Asking the Right Questions > M. Neil Browne Stuart M. Keeley > Bowling Green State University"],
 "expected_no_evidence": false}
{"id": "g-ab-01", "intent": "abstention",
 "query": "What is the capital of Atlantis?",
 "expected_chunk_ids": [], "expected_sections": [], "expected_no_evidence": true}
```

### The Corpus Fingerprint — the validity proof

```go
// internal/eval/fingerprint.go
func ComputeFingerprint(contentHashes []string) string {
	sorted := append([]string(nil), contentHashes...)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(h[:])
}
```

SHA-256 over the **sorted** `ContentHash` values of the indexed chunks (sorted so indexing order never matters). The runner *hard-fails before executing any query* if the live index's fingerprint mismatches the Gold Set's declared value (per-document for schema-1.2 sets). Meaning: **a benchmark against a stale or partial index never happens** — the fingerprint is the proof that the benchmark measured the corpus it claims.

### The runner and metrics

`Runner.Run(ctx, goldset)`: verify fingerprint → per query: retrieve → record retrieved IDs + scores + stats → compute per-query metrics → aggregate.

All metrics use **binary relevance** (a chunk is relevant or not — no graded relevance):

- `RecallAtK` = relevant-in-top-k / total relevant
- `PrecisionAtK` = relevant-in-top-k / k
- `MRR` = 1/(rank of first relevant), 0 if none
- `NDCGAtK` = DCG/IDCG with binary gains: `Σ 1/log2(i+2)` over top k, IDCG from the ideal ordering
- `NoEvidencePrecision` = fraction of abstention queries that retrieved 0 chunks

Aggregation: Recall/Precision/MRR/NDCG averaged over **non-abstention queries only**; abstention handled separately. Per-query detail goes into `Report.PerQuery` (retrieved IDs, scores, stats) so any aggregate can be recomputed from the raw artifact.

The runner also mirrors the engine's abstention semantics: no gate call on empty retrieval; gate retries match `MaxGateAttempts`; `--gate-runs N` repeats gate evaluation and takes the **median decision** (order: supported < unsupported < gate_error; even counts take the lower median) — the M7 BULGU-2 stabilization against LLM variance. Retrieval runs once; its metrics are unaffected by gate runs.

### The gates that decide

The M-milestones each shipped a frozen acceptance rule (this is the pattern to copy):

| Milestone | Gate rule (frozen before the benchmark) |
|---|---|
| M4 fusion | Sweep policies on Gold Set v2; production keeps `Balanced`; `DenseBiased` exists as a calibrated policy, never runtime-selected (ADR-0036) |
| M6 orchestration | `ComparisonTopK` evidence budget calibrated to 8; TopKOverride applies only to comparison intent (ADR-0037) |
| M7 graph | ADR-0040: entity recall ≥ dense×1.05 **and** MRR ≥ dense; ≤5% regression elsewhere; zero abstention leak. Weight sweep {0.5, 1.0, 2.0} → froze 1.0 (ADR-0041); rejection → `GraphWeight=0` keeps byte-identical dense behavior |
| M8 rerank (in progress) | MPI: nDCG@5 Δ ≥ +1pp over baseline; MAR: MRR Δ ≥ −0.5pp, verified Δ ≥ −1pp; abstention = hard invariant; budgets frozen in the manifest; "none accepted" closes M8 |

And the 5%-tolerance regression rule everywhere: a change that regresses any metric more than 5% vs the committed baseline fails the gate regardless of its headline gains.

## The M8 reranking probe (current state, for orientation)

M8 adds a second-stage reranker behind the retriever seam (`RerankedRetriever` wraps any `seam.Retriever`, requests candidate budget N internally, returns the caller's TopK; `Reranker` seam with an *ordering-only* contract — scores are informational, never cross-adapter comparable; tie-break ChunkID ASC via `StabilizeOrdering`). The probe (candidate artifact + kill gate + manifest, `arc eval probe collect|run`) measures BGE-reranker-v2-m3 at N ∈ {20, 50, 100} against the frozen baseline, with the artifact as an immutable re-runnable record. Acceptance freezes (model, N) and wires `WithGraphRetriever(NewRerankedRetriever(graphFusion, config))`; rejection closes M8 with production unchanged. (ADRs 0043–0046; the warm-up probe run on this corpus rejected — rerankers degraded ranking on the gold set with the payload-fixed content — the gate working as designed.)

## The agent layer (honest status)

`AgentEngine` (ADR-0017) is a **scaffold, not the full vision**: `ExecuteResearch` validates policy, emits a hardcoded 3-line plan summary, and executes every tool in map order up to `MaxSteps`. The `Tool` seam exists (`Name`/`Description`/`Execute`; `KnowledgeTool` wraps `AnswerEngine.Answer`). `MaxToolCalls`/`TokenBudget`/`RequireApproval` are struct fields, not behaviors — there is no Planner, no ApprovalSeam yet. The QAJob async state machine (ADR-0014: `Pending → Planning → Retrieving → Generating → Verifying → Completed`) is planned, not wired.

## Planned (from ADRs, not yet built — know these so you don't rebuild them wrongly)

- **Async indexing/QA jobs** — state machines exist (`internal/indexing/job`, `internal/qa/job`), only sync paths live.
- **NLI entailment (Phase 2 verification)** — the `EntailmentChecker` seam exists, unused.
- **Anthropic/other LLM providers** — the seam accepts them; only the OpenAI-compatible gateway ships.
- **`ExtractedFilter` population** in the analyzer (currently never filled) — the natural place for "give me page-12 tables" queries.
- **`SectionPathPrefix`/`IndexedAfter` filters in Qdrant** — validated and honored in-memory, silently dropped on Qdrant (a known gap).
- **Graph edges/Traverse** — v1 is entity-only; the seam documents the future.
- **PDF engine swap** — research (pdf-inspector/anydoc) exists in `docs/research/`; a swap must preserve `json_layout.pages` synthesis (PageMap) or accept chunk-page loss.

## Why This Matters

The evaluation framework is the *only* thing that lets ARC evolve safely. It converts every architectural opinion into a falsifiable claim. It is why frozen constants exist (they are benchmark outputs, not config). And it is the reason a rejected M8 probe — a milestone that "failed" — is considered a *correct* outcome: the system measured, the gate held, and production stayed byte-identical.

## What Would Break Without It

No Gold Set → no versioned truth; changes ship on vibes. No fingerprint → benchmarks silently run against stale indexes and the numbers lie. No gates → regressions accrete invisibly (the exact failure mode of naive RAG). Without the runner's artifact discipline, the M4–M8 calibration history — every sweep, every freeze, every rejection — would be unreproducible folklore instead of evidence.

---

# Closing: the ten-minute mental model

1. **A book enters as bytes and exits as ranked evidence.** PDF → Firecrawl service → `PDFInspectionResult` (tree, chunks, pages, assets, diagnostics) → Qdrant points (dense + sparse + payload) → retrieved chunks → gated, cited `Answer`.
2. **Every arrow crosses an interface.** `Client`, `EmbeddingProvider`, `VectorStore`, `Retriever`, `Reranker`, `EvidenceGate`, `LLMProvider` — each one documents a contract, and each contract is benchmarked.
3. **Chunks are the atomic unit.** They carry section paths, pages, citations, hashes, and links — everything retrieval and QA needs, precomputed once at ingestion.
4. **Identity is structural; change is hashed.** Point IDs = `SHA256(DocumentID:SectionPath:ChunkOrder)`; change detection = `IndexSignature = SHA256(ContentHash:Provider:Model:Version:SchemaVer)`. Diffing is cheap because both are digests.
5. **Scores never mix; ranks do.** RRF (k=60) fuses dense, sparse, and graph streams by rank, not scale — and the weights are frozen benchmark artifacts.
6. **Abstention is a feature.** Empty retrieval and an unsupported gate decision both return `no_evidence` — an honest answer is one that knows when it has nothing.
7. **Nothing ships on vibes.** Gold Set + Corpus Fingerprint + frozen acceptance rules + 5% regression tolerance — and a rejected milestone is a normal outcome.
8. **Determinism is a contract.** Tie-breaks, term IDs, CI seeds, and re-runs all exist so that any claim in any benchmark can be replayed.

*"Measurement decides, assumption never does." — the one sentence that explains 46 ADRs.*

---






