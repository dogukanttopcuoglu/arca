# Document-level retrieval: profile points + document_overview intent

Document-level questions ("bu kitap ne anlatıyor", "what is this book about", "yazar kim") failed with `no_evidence`: chunk-level semantic search cannot answer them — the generic query embedding ranks below the min-score, and the top-5 mid-section chunks are (correctly) rejected by the EvidenceGate. The enrichment layer already produces the needed material (TitleResolver, SummaryPass, KeywordExtractor) but drops it at indexing. We decided to (1) persist a deterministic **Document Profile Point** per document at index time (`content_type=document_profile`, content = title/author/extractive summary/key topics/section list, embedded, diff-lifecycle-aware, **excluded from the corpus fingerprint** — the fingerprint covers indexed *chunks* and a profile is not a chunk), (2) add a **document_overview** intent to the rule-based analyzer (capped EN+TR pattern set, structured patterns first, `özet`+bölüm/konu and "who is X" explicitly excluded), and (3) route it through a **Document Overview Retriever** implementing the existing `Retriever` seam — profile + first real content chunks (`chunk_order > 1`, front-matter excluded at retrieval time) with a document filter, all profile points without one. Graph, decomposition, and hybrid paths stay disabled for this intent; ContextBuilder, EvidenceGate, and LLM flow are untouched.

Status: accepted (2026-08-07 — benchmark-gated activation complete: gold set v5 overview slice recall@5 1.000 / MRR 1.000, v3.1 + E1 probe regression ≤ 5%, EvidenceGate active; see `docs/benchmarks/M9_VALIDATION.md`).

## Considered options

- **LLM summarization for the profile**: rejected for this change — SummaryExtractor seam (ADR-0024) already anticipates a later LLM strategy; the extractive profile is deterministic and sufficient for document-identity questions.
- **Section-name heuristics** ("Introduction" prefix) for the overview context: rejected — section names vary per book ("Introduction" does not exist in rick-rubin); `chunk_order > 1` selection is corpus-agnostic and excludes the known front-matter (copyright page is order 1).
- **Filtering front-matter noise at the gate**: rejected — noise is excluded at retrieval time; the gate must not be asked to clean input.
- **Treating the profile as a chunk** (participating in fingerprints and chunk benchmarks): rejected — it would break every existing gold set and conflate document metadata with corpus content.
- **New intent classifier / embedding-based detection now**: rejected — rule-based capped patterns cover the benchmarked forms; the exemplar-based matcher is the Phase 2 path when patterns would exceed their cap (see the query-understanding evolution plan).

## Consequences

- All indexed documents must be re-indexed to create profile points (idempotent, diff-safe).
- `goldset_v5.json` declares expectations via `expected_retrieval_points` (profiles are not chunks; ADR-0027 field generalized).
- `ContentTypeDocumentProfile` joins the content-type allowlist for `MetadataFilter` validation.
- If the profile content changes, the diff engine re-upserts the single point — never chunk content.
