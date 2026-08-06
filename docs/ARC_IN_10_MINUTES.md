# ARC in 10 Minutes

> The canonical onboarding document. Read this first, then open the engineering handbook.
> Reading time: 8–10 minutes. No code, no JSON — just the mental model.

---

## 1. ARC in One Sentence

**ARC turns unstructured documents into verifiable knowledge that can answer questions with citations.**

Not "answers questions" — **verifiable** answers: every claim the system makes can be traced back to a page number in a real book, and when the books can't answer, ARC says so instead of making something up.

---

## 2. The Problem

A user uploads *The Creative Act* by Rick Rubin (248 pages) and asks:

> "What does Rick Rubin mean by tuning in?"

Naive RAG (the "vibe pipeline") fails in five predictable ways:

1. **It can't read the book properly.** PDFs are hostile: multi-column layouts, tables drawn as vector graphics, fonts with broken encodings, scanned pages with no text at all. A naive extractor produces garbled or missing content — silently.
2. **It destroys structure.** It slices the text into fixed-size windows. A window boundary lands mid-argument, and "Tuning In" as a *section* disappears — it's just a heading somewhere.
3. **It loses pages.** Chunk #47 doesn't know it came from page 12. The answer can't cite anything.
4. **It mixes everything into one bag.** Dense (semantic) and lexical (exact-word) matches find different things; naive systems pick one, or naively sum scores that live on different scales.
5. **It hallucinates confidently.** With no evidence gate, the LLM answers "what is tuning in?" from its own priors — confidently, fluently, and wrongly.

ARC exists to make every one of these failures *measurable* and *fixable* — by turning document understanding into a pipeline of verifiable steps.

---

## 3. The Entire Pipeline

```
User asks a question
        │
        ▼
   ┌────────────────────────────────────────────┐
   │                THE BOOK                     │
   │        (PDF: text, tables, pages)          │
   └──────────────────┬─────────────────────────┘
                      ▼
   ┌────────────────────────────────────────────┐
   │  EXTRACTION (Firecrawl service)            │
   │  PDF → clean Markdown + page layout        │
   └──────────────────┬─────────────────────────┘
                      ▼
   ┌────────────────────────────────────────────┐
   │  SEMANTIC RECONSTRUCTION                   │
   │  Markdown → heading tree with page numbers │
   └──────────────────┬─────────────────────────┘
                      ▼
   ┌────────────────────────────────────────────┐
   │  CHUNKING (hierarchical semantic)          │
   │  tree → self-contained 400–700-token units │
   └──────────────────┬─────────────────────────┘
                      ▼
   ┌────────────────────────────────────────────┐
   │  EMBEDDING + INDEXING (differential)       │
   │  chunks → dense + sparse vectors → Qdrant  │
   └──────────────────┬─────────────────────────┘
                      │         ▲
                      │         │
   ┌──────────────────▼─────────┴──────────────┐
   │  RETRIEVAL (dense + sparse + entity graph) │
   │  question → top-5 ranked chunks            │
   └──────────────────┬─────────────────────────┘
                      ▼
   ┌────────────────────────────────────────────┐
   │  ANSWER ENGINE                             │
   │  context → evidence gate → LLM → verify    │
   └──────────────────┬─────────────────────────┘
                      ▼
        Answer with citations — or an honest
        "no evidence" abstention
```

Two halves, one bridge:

- **Ingestion** (top half): documents in → chunks in a vector store. Deterministic — the same PDF always produces the same chunks and hashes.
- **QA** (bottom half): question in → cited answer out. Reads the vector store; never touches documents.

The bridge is the **vector point**: every chunk is persisted with its section path, page numbers, content type, citations, and versioned index signature.

---

## 4. The Journey of One Document

*The Creative Act* arrives as `rick-rubin.pdf`.

**1. Extraction.** The Firecrawl service (a separate Node.js microservice in Docker) parses the PDF: detects it's text-based, reconstructs reading order, converts to clean Markdown, and returns a page-by-page layout. 248 pages become Markdown with page markers.

**2. Semantic reconstruction.** ARC walks the Markdown with a heading-aware parser: `### Tuning In` becomes a tree node; every paragraph beneath it is recorded as *belonging* to that section, on that page. The result is a heading tree — 98 top-level sections, each knowing its page range.

**3. Chunking.** Each section is cut into self-contained chunks of 400–700 tokens: paragraphs stay whole, tables are never split, and a section with several chunks gets a synthetic parent chunk that links its children. Every chunk carries its section path (`Tuning In`), page numbers (11–14), citations, content type, and a content hash. Result: **196 chunks** with stable IDs like `rick-rubin/tuning-in/002`.

**4. Enrichment (sidecar).** Rule-based passes derive the title, detect the language, and extract entities, keywords, concepts, and relations — "Rick Rubin founded Def Jam Recordings" becomes a typed relation that later feeds the entity graph. Every failure here becomes a warning, never a pipeline failure.

**5. Embedding + indexing.** The worker compares each chunk's *index signature* against what's already stored: identical chunks are skipped entirely (no embedding calls), changed chunks are re-embedded, removed chunks are deleted. New chunks are embedded (768-dim vectors) and upserted into Qdrant alongside BM25 sparse vectors — one collection, both representations, plus the full metadata payload.

**6. Retrieval.** "What does the book say about Rick Rubin?" is classified as an *entity* question, which opens the graph gate: the retriever fuses a dense semantic search with a sparse lexical search and an entity-graph lookup (Rick Rubin's name maps to the chunks that mention him) using rank fusion — top-5 chunks, deterministically ordered.

**7. Answer engine.** The five chunks are assembled into a context window with immutable citation markers (`[Ref 1]`…`[Ref 5]`). An evidence gate asks: *do these sources actually answer the question?* If yes, the LLM generates an answer that must cite its claims. A verifier then checks every `[Ref N]` actually exists. The answer lands with citations to specific pages — or, if the evidence gate says no, ARC abstains with an honest "the sources don't cover this" rather than inventing one.

---

## 5. Major Components

### Extraction — *the reading engine*

- **Problem:** PDFs are hostile and parsing them is a multi-year project.
- **Input:** raw PDF bytes.
- **Output:** clean Markdown + page layout + metadata.
- **Responsibility:** ARC outsources the parser to Firecrawl's service behind a strict HTTP contract with retries and fail-fast validation, so ARC owns *analysis*, not parsing.

### Semantic Reconstruction — *the structure engine*

- **Problem:** Markdown is a flat string; a book is a hierarchy.
- **Input:** Markdown + optional structured layout JSON.
- **Output:** a heading tree (`SemanticNode`s with levels and page numbers).
- **Responsibility:** turn "### Tuning In" plus paragraphs into "a section called Tuning In, pages 11–14, containing these parts."

### Chunking — *the meaning engine*

- **Problem:** LLMs can't read 248 pages; retrieval needs self-contained units.
- **Input:** semantic tree + Markdown.
- **Output:** `KnowledgeChunk`s: 400–700 tokens, section path, pages, citations, content type, parent/child links, content hash.
- **Responsibility:** make each chunk a *semantically intact* unit — never split a table, never merge two sections, always know its pages.

### Embedding — *the meaning-to-numbers engine*

- **Problem:** machines don't understand meaning; vectors approximate it.
- **Input:** chunk Markdown.
- **Output:** dense vectors (768-dim, local Ollama) + BM25 sparse vectors.
- **Responsibility:** two complementary representations — semantic similarity and exact lexical match — both persisted in one Qdrant collection.

### Indexing — *the diff engine*

- **Problem:** re-uploading a fixed book must not re-embed everything.
- **Input:** new `PDFInspectionResult` + existing points.
- **Output:** a diff plan — new / changed / unchanged / deleted — executed atomically.
- **Responsibility:** identity is *structural* (document + section + order), change detection is *hashed* (a signature over content + model + version). Unchanged chunks cost zero embedding calls.

### Retrieval — *the finder*

- **Problem:** one search strategy misses what another finds.
- **Input:** `RetrievalQuery` (text, top-k, filters, min score).
- **Output:** ranked `SearchResult`s with scores, content, and metadata.
- **Responsibility:** fuse dense + sparse + entity-graph streams by *rank agreement* (RRF), never by raw scores; honor metadata filters; abstain (empty result) when nothing clears the threshold.

### Answer Engine — *the verifier*

- **Problem:** LLMs hallucinate when forced to answer.
- **Input:** query + retrieved chunks.
- **Output:** an `Answer` with citations, a verification report, and an explicit status: `verified`, `unverified`, or `no_evidence`.
- **Responsibility:** analyze intent (comparison → decompose; entity → graph), assemble a citation-marked context, gate the evidence *before* generation, generate with mandatory citations, verify every reference afterwards, and abstain honestly when evidence is missing.

---

## 6. Current State

| Capability | Status |
|---|---|
| Extraction (Firecrawl service) | ✅ Built |
| Semantic tree + page mapping | ✅ Built |
| Hierarchical chunking | ✅ Built |
| Asset extraction (tables, citations, …) | ✅ Built |
| Enrichment (title, language, entities, keywords) | ✅ Built (rule-based) |
| Differential indexing + IndexSignature | ✅ Built |
| Dense + sparse (BM25) + hybrid RRF | ✅ Built |
| Entity graph + graph fusion (M7) | ✅ Built (frozen weight 1.0) |
| Answer engine: analyze → gate → generate → verify | ✅ Built (Phase 1 verification) |
| Evaluation framework (Gold Set, fingerprint, gates) | ✅ Built (v3, 37 queries, 5 books) |
| Reranking (M8) | 🚧 Probe rejected on the gold set — gate held, not activated |
| NLI entailment (Phase 2 verification) | 🚧 Seam exists, no implementation |
| Async jobs (indexing / deep research QA) | 🚧 State machines exist, sync-only paths live |
| Agent engine (planner, tools, approvals) | 🚧 Scaffold (tool seam works) |
| Production deployment / multi-provider LLMs | ⏳ Planned |

The pattern to notice: **everything "✅" earned its place through a benchmark gate; nothing is marked done on vibes.**

---

## 7. Guiding Principles

**Measurement over assumptions.** Every retrieval-affecting change must prove itself against a fixed, versioned benchmark (the Gold Set) before reaching production — and a rejected change is a *normal, correct* outcome. This is why there are 46 ADRs and why the codebase is littered with frozen constants: they are benchmark outputs, not opinions.

**Deep seams.** Every subsystem is an interface with a documented contract: extraction, embedding, retrieval, reranking, evidence gating, LLM generation. Seams make systems testable, swappable, and independently measurable — and they keep domain code from coupling to specific vendors.

**Determinism is a contract.** Same query → same ranking. Score ties break by chunk ID. Term IDs come from sorted vocabularies. Bootstrapped intervals use fixed seeds. If a benchmark can't be replayed, it's worthless — so the system is built to be replayed.

**Graceful degradation.** A corrupt page, a failed table, an OCR-less scan — none of these fail the pipeline. They produce `partial_success` diagnostics with warnings and skipped pages. Only structural failure (encrypted, invalid) fails. ARC never hides imperfection; it reports it.

**Frozen policies.** Fusion weights, graph weights, min-score thresholds, evidence budgets — all are calibrated offline and frozen. Runtime code never tunes. When a better policy is found, it wins its own benchmark first; the old frozen value stays until then.

**Non-destructive enrichment.** Everything that adds knowledge to a document (title resolution, entities, page mapping) can fail without failing the pipeline. Enrichment is additive; its failures are warnings with diagnostics, never silent data loss.

---

## 8. Where To Go Next

| Document | What it's for |
|---|---|
| **`docs/engineering-handbook.md`** | The full 1,300-line deep dive: every subsystem with worked examples, verbatim data structures, trade-offs, and "what would break without it". Read this next. |
| **`docs/adr/`** (0001–0046) | The decision log. Each ADR records *why* a design exists, what it costs, and what it freezes. Read ADR-0027 (evaluation) and ADR-0008 (indexing) first; then read ADRs in the order you explore subsystems. |
| **`internal/eval/testdata/goldset_v3.json`** | The evaluation truth: 37 real queries across 5 real books with declared expected chunks and abstention expectations. The corpus fingerprint makes benchmarks against a stale index impossible. |
| **`docs/benchmarks/`** | The evidence trail: baseline reports, fusion sweeps, M7 calibration and closeout. This is what "measurement decides" looks like as data. |
| **`docs/research/`** | Exploratory research (e.g. reranking architectures) that informed milestones — including things that were evaluated and *not* adopted. |

---

*Welcome to ARC. The books are already loaded — ask a question and see the machine prove it.*
