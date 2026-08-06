# Heading-Aware Embedding — Design Review & Benchmark Plan

> Status: design review (no implementation)
> Date: 2026-08-06
> Trigger: RCA of "What does Rick Rubin mean by tuning in?" returning `no_evidence` (Tuning In chunks ranked 56/62, below MinScore 0.6; heading never enters the embedding input — worker embeds only `ContentMarkdown`, worker.go:160).

---

## 1. Representation Design Review

### What embedding sees today

```
embedding_input = ContentMarkdown
```

Evidence (worker.go:160, 204, 212): both dense (`EmbedDocuments(texts)`, `texts[bIdx] = chk.ContentMarkdown`) and sparse (`encoder.Encode(chk.ContentMarkdown)`) see only the chunk body. The heading exists **only** in `VectorMetadata.SectionPath` (payload) — invisible to both representations. This is why section-title queries fail: "tuning in" appears in no indexed text.

### The four candidates

| | A. ContentMarkdown (status quo) | B. SectionTitle + Content | C. Full SectionPath + Content | D. BookTitle + SectionPath + Content |
|---|---|---|---|---|
| **Example input** | `Think of the universe...` | `Tuning In\n\nThink of the universe...` | `Creativity > Tuning In\n\nThink of...` | `The Creative Act > Creativity > Tuning In\n\nThink of...` |
| **Semantic retrieval** | Fails section-title queries (RCA) | Solves same-name sections *within* a book only | Solves nested/ambiguous section names; disambiguates same-named sections across books poorly | Full disambiguation; multi-document corpus (5 books) — "Introduction" ×5 collision resolved |
| **Lexical retrieval (BM25)** | "tuning" absent → never matches | "tuning" enters index → matches | Same + parent headings indexed | Same + book title indexed; generic headings get low IDF (high DF) |
| **Embedding quality** | Chunk-internal semantics only | Single heading biases the vector toward one term | Hierarchical context; top headings dilute the signal *slightly* (longer prefix) | Prefix ~15–25 tokens vs ~400–700 body: <5% of input; acceptable dilution |
| **Chunk independence** | Maximal (each chunk self-contained) | Good | Good (prefix is provenance, not body) | Good |
| **Duplication risk** | None | Same-named sections in one book collide | Low within book; cross-book duplicates remain | Lowest — book title breaks ties |
| **Token cost per chunk** | 0 | ~2–5 | ~5–15 | ~15–25 (worst case: deep TOC paths) — <5% of the 400–700 budget; well under the 8,192 embedding limit |
| **Wrong-match risk** | Section-title queries always wrong | Medium (same-name sections) | Low | Low; residual risk: book-title term appears in queries about the book itself — acceptable, it *is* the book |

**Verdict per axis:** A fails the goal. B is a half-fix (multi-section books with repeated names, and ARC's corpus has exactly this: 5 books, generic headings like "Introduction", "References"). C fixes within-book ambiguity. D additionally fixes cross-book ambiguity — which is the corpus reality (goldset v3 = 5 documents) — and matches Azure's documented guidance (below).

---

## 2. Industry Review

### Anthropic — Contextual Retrieval (verified, primary source)

Source: [anthropic.com/news/contextual-retrieval](https://www.anthropic.com/news/contextual-retrieval) (Sep 2024). Anthropic *prepends chunk-specific context* (LLM-generated, 50–100 tokens) to each chunk **before embedding and before BM25 indexing** — the same architectural move under review, applied to both representations:

> "Contextual Embeddings reduced the top-20-chunk retrieval failure rate by 35% (5.7% → 3.7%). Combining Contextual Embeddings and Contextual BM25 reduced the top-20-chunk retrieval failure rate by 49% (5.7% → 2.9%)."

Relevant negative result from the same post (also verified):

> "Other proposals include adding generic document summaries to chunks (we experimented and saw very limited gains)"

Implication for ARC: context *quality* matters — a one-line LLM-synthesized context beats a generic summary. Heading/SectionPath is a cheap, deterministic, zero-LLM-cost context — weaker than Anthropic's LLM context, but the deterministic tier between "no context" (current) and "LLM context" (cost, determinism break — violates ARC's ingestion determinism). Anthropic's headline numbers bound the *potential*: our fix targets a subset of that 35% failure reduction.

### Azure AI Search — chunking guidance (verified, primary source)

Source: [learn.microsoft.com/azure/search/vector-search-how-to-chunk-documents](https://learn.microsoft.com/en-us/azure/search/vector-search-how-to-chunk-documents). Under "Custom combinations":

> "when dealing with large documents, you might use variable-sized chunks, but also **append the document title to chunks from the middle of the document to prevent context loss**."

Directly documents the D-style pattern (document title + chunk) as Microsoft's guidance for large documents.

### LlamaIndex — the counter-pattern (verified, primary source)

Source: [docs.llamaindex.ai — Node Parser Modules, SentenceWindowNodeParser](https://docs.llamaindex.ai/en/stable/module_guides/loading/node_parsers/modules/):

> "The resulting nodes also contain the surrounding 'window' of sentences around each node in the metadata. **Note that this metadata will not be visible to the LLM or embedding model.**"

A respected framework deliberately keeps retrieval embeddings *narrow* and enriches the LLM context instead (via `MetadataReplacementNodePostProcessor`). It is the opposite design pole: narrow embedding + rich generation context. ARC already follows half of this (ContextBuilder includes SectionPath in the prompt's `[Ref N]` blocks); the RCA shows the narrow-embedding half is too narrow for section-title queries. Industry is split between "enrich the embedding input" (Anthropic, Azure) and "keep embeddings narrow, enrich the prompt" (LlamaIndex window parser) — the split is corpus-dependent, which is exactly why ARC must decide by benchmark, not by authority.

### Not verifiable (fetched, not found)

- **LangChain**: docs index redirect; `MarkdownHeaderTextSplitter` page unreachable — no claim made.
- **Haystack**: `docs/preprocessing` and `docs/cleaning` → 404 — no claim made.
- **OpenAI Cookbook**: chunking page → 404 — no claim made.
- **Google Vertex AI Search**: chunking pages → 404 — no claim made.

---

## 3. Side-Effect Analysis

### Queries that should improve

| Query type | Example | Mechanism |
|---|---|---|
| Section-title search | "What does the book say about tuning in?" | Title tokens now in dense+sparse input |
| Explain section | "Explain what Rubin means by listening" | Same |
| Chapter navigation | "Summarize the chapter about the unseen" | Same |
| Cross-book disambiguation | "What does book A say about Introduction vs book B?" (D only) | Book title in input |
| Entity-in-section | "What does the book say about Rick Rubin in Tuning In?" | Both signals align |

### Queries at regression risk

| Query type | Risk | Why | Mitigation |
|---|---|---|---|
| Generic semantic search | Low | Prefix is ≤5% of input; body dominates the vector | Gold Set v3 concept/single_fact slices as regression canary |
| Paragraph-level similarity | Low–medium | Two chunks in the *same* section share a prefix → artificially closer; within-section ranking flattens | Measure per-query nDCG (rank-sensitive), not just recall |
| Concept retrieval | Low | Concept queries are body-driven | Existing intent slices |

### Trade-off table

| Dimension | Gain | Cost |
|---|---|---|
| Section-title recall | Large (RCA case: rank 56 → expected top-5) | Prefix dilution of body semantics |
| Cross-book precision | Large in 5-book corpus (D) | Longer inputs (+15–25 tokens) |
| Within-section ranking | Slight flattening | Prefix identical within section |
| Determinism | Preserved (no LLM in ingestion) | — |
| Index migration | — | Full re-index required (see §5) |

---

## 4. Chunk Identity Review — embedded vs metadata

| | SectionPath embedded | SectionPath metadata-only (today) |
|---|---|---|
| Retrievable by title | ✅ both dense and sparse | ❌ not at all (proven by RCA) |
| Filtering | Also available via `MetadataFilter` (unchanged) | ✅ only mechanism today |
| Storage cost | +tokens in input only (no payload change) | Payload already carries it |
| Chunk body purity | Body unchanged; only the *input* to embedding changes | — |
| Identity semantics | `ContentHash` stays content-only (see §5) | — |

**Conclusion:** not either/or — ARC keeps `SectionPath` in payload *and* adds it to the embedding input. Metadata keeps its filter role; the embedding input gains provenance. The two mechanisms are complementary, and the payload schema does not change at all (the change is confined to the worker's embedding-text construction + version bump).

---

## 5. Diff Engine Impact

### Current signature inputs (verified, model/signature.go + worker.go:171)

```
IndexSignature = SHA256( ContentHash : EmbeddingProvider : EmbeddingModel : EmbeddingVersion : ChunkSchemaVer )
ContentHash    = SHA256( NormalizeMarkdown(ContentMarkdown) )     ← content-only, by design
EmbeddingVersion literal = "1.0.0"  (worker.go:100, 171; hardcoded)
```

### What changes and what must change

| Question | Answer | Evidence/reasoning |
|---|---|---|
| Does `ContentHash` change? | **No — must not.** | Hash is content identity; embedding input is not content. Also: Corpus Fingerprint = SHA256 over sorted `ContentHash` values — changing ContentHash semantics would invalidate every committed Gold Set fingerprint (v1.1/v2/v3) and break benchmark validity. |
| Does the payload change? | No. | `SectionPath` already in payload; `content_markdown` stays the raw body. |
| Does `IndexSignature` change? | **Yes — required.** | Same hash + same provider/model with a new input would produce an identical signature but a *different vector* → diff would wrongly mark chunks unchanged and keep stale vectors. |
| Is `EmbeddingVersion` bump sufficient? | **Yes — this is the designed mechanism.** | `1.0.0 → 1.1.0` changes every signature → all 3017 chunks become `Modified` → full re-embed. The version field exists precisely for this ("the model must be a pinned tag so IndexSignature stays stable" — the same discipline applies to the input representation). |
| Is full re-index mandatory? | **Yes, and it is the intended cost.** | One-time Ollama re-embed of 3017 chunks (~2–4 min locally). No partial migration exists — by design, a representation change is atomic. |
| Does the sparse side need the same? | Yes. | Sparse vectors encode the same text; the BM25 index must be rebuilt with the new input (same re-index covers it). |

### Architectural conclusion

The diff engine is *ready* for this change — no code change in `diff/` is required. The version-bump-to-invalidate pattern is the frozen mechanism. The only new decision is the version literal's meaning: `EmbeddingVersion` must be documented as "representation version" (input schema), not just "model adapter version".

---

## 6. Benchmark Design

### Principle

One example ("Tuning In") must not decide. The change must prove itself on a *query class*, across all 5 books, with the standard gate discipline (ADR-0027: fingerprint-gated, 5% regression tolerance, frozen thresholds before measurement).

### New gold set category: `heading` (goldset v4, additive)

A new intent category `heading` added to the Gold Set schema (v1.3), ~15–20 queries across all 5 books, human-curated from real section titles of the indexed corpus — **same corpus, so the existing Corpus Fingerprint still holds** (chunk hashes unchanged; see §5). Query shapes:

1. **Section-title search** — "What does the book say about Tuning In?" (section title verbatim)
2. **Section-title paraphrase** — "How does Rubin describe being in tune with the world?" (title NOT verbatim — tests semantic reach, not lexical)
3. **Explain section** — "Explain what Rubin means by listening"
4. **Chapter navigation** — "Summarize the chapter about the unseen"
5. **Cross-book disambiguation (D-critical)** — "What does *Asking the Right Questions* say about Introduction?" where ≥2 books have a section named "Introduction" — expected chunks must come from the *named* book only
6. **Abstention guard** — "What does the book say about [real-sounding but absent section title]?" (`expected_no_evidence: true`)

Expected chunks: real chunk IDs from the indexed corpus (ADR-0027 rule: no synthetic chunks, no injected facts).

### Metrics — which one is meaningful

| Metric | What it measures | Verdict for this change |
|---|---|---|
| Recall@5 | Did *any* relevant chunk make top-5 | Minimal bar; insufficient alone (a degraded ranker still hits recall) |
| **Hit Rate@5** | ≥1 relevant in top-5 | Good coarse guard, weakest signal |
| **MRR@5** | Position of *first* relevant | Meaningful: title queries either rank #1 or fail |
| **nDCG@5** | Full rank quality over expected set | **Primary** — rank-sensitive, matches existing gates |

Primary: **nDCG@5** (consistent with M7/M8 MPI). Secondary: MRR@5. Coarse guard: Hit Rate@5 reported, not gating. Recall@5 is a diagnostic (recall is bounded by retrieval, rerank-free — same as M7).

### Gate rule (frozen before measurement)

- **MPI**: nDCG@5 delta ≥ +1 pp vs current representation **on the `heading` slice** (baseline measured first on the same slice).
- **MAR**: ≤5% regression on goldset v3's existing intent slices (concept, single_fact, comparison, entity, abstention) — the generic-retrieval regression canary (§3).
- Abstention: zero leak (hard invariant) — heading-queries' abstention behavior identical.
- Optionally A/B/C/D representations measured side-by-side in one probe (M8 artifact-style: same queries, 4 representations, one report) — D wins only if it beats C within the tolerance band by the smallest-N-style rule (here: smallest-input rule).

---

## 7. Risk Analysis

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| **Embedding drift** — all vectors change at once (full re-index); old query behavior not reproducible | Certain (by design) | High | Version bump + one-atomic migration; benchmark before/after on the SAME goldset v3 (fingerprint-valid); commit manifest with both representations' numbers |
| **Generic retrieval regression** — prefix dilutes body semantics on concept/single_fact queries | Medium | Medium | Existing v3 intent slices act as regression canary; MAR ≤5% gate; if it regresses, fall back to C (shorter prefix) |
| **Within-section ranking flattening** — same-section chunks share a prefix, dense ties tighten | Medium | Low–medium | nDCG@5 is rank-sensitive and will catch it; tie-break ChunkID ASC keeps determinism |
| **Duplicate bias (cross-book)** — "Introduction" ×5 books collide on title tokens | Medium (without D) / Low (with D) | Medium | D's book-title prefix; BM25 IDF naturally down-weights high-DF generic headings |
| **Heading overfitting** — benchmark tuned to heading queries, generic retrieval forgotten | Low | Medium | Gate requires BOTH slices (heading MPI + v3 MAR) — a heading-only win is insufficient |
| **Book-title quality dependency (D)** — TitleResolver output is sometimes imperfect (rick-rubin resolved to "Robert Henri") | Medium | Low | Even an imperfect title disambiguates across books; fallback chain: resolved title → DocumentID slug; note in benchmark manifest |
| **Token cost creep** — deep TOC SectionPaths | Low | Low | Cap prefix length (e.g. last 3 path segments) — a pure implementation detail, benchmark-agnostic |

---

## 8. Final Recommendation

### ✅ D — BookTitle + SectionPath + ContentMarkdown

Reasons, in order of weight:

1. **The corpus is multi-document.** goldset v3 spans 5 books; generic section titles ("Introduction", "References", "About the Author") recur across books. Only a book-level prefix resolves cross-book ambiguity — the exact failure class B and C leave open, and the exact class Azure documents guidance for ("append the document title to chunks... to prevent context loss").
2. **It fixes the RCA case and its whole query class** (section-title search/explain/navigation) with a deterministic, zero-LLM-cost input change — in line with Anthropic's contextual-embeddings direction, at the cheap deterministic tier ARC can afford without breaking ingestion determinism (ARC's fingerprint discipline forbids LLM-at-index-time).
3. **It is a confined change**: worker embedding-input construction + `EmbeddingVersion` bump `1.0.0 → 1.1.0`. No payload schema change, no `ContentHash` change (preserves all Gold Set fingerprints), no diff-engine change — the existing version-invalidation mechanism does the migration for free.
4. **Both representations benefit**: BM25 gains title tokens (fixing the lexical side of the RCA — "tuning" becomes an indexable term), dense gains provenance.
5. **Risk is bounded by the existing gate machinery**: heading-slice MPI (+1 pp nDCG@5) + v3-slice MAR (≤5%) + abstention hard invariant; if D's longer prefix regresses generic retrieval, C (SectionPath-only) is the measured fallback, and the probe report decides between them — not authority.

**Constraint (non-negotiable):** `EmbeddingVersion` semantics must be documented as "embedding input representation version" before the bump, so future representation changes reuse the mechanism without ambiguity.

---

*Decision procedure: this document is the design review; the benchmark (probe with A/B/C/D on goldset v4's heading slice + v3 MAR slices) is the gate. Implementation starts only after the probe accepts.*
