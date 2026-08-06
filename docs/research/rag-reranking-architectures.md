# RAG Reranking Architectures — Research Report

Status: research only (no implementation proposed)
Date: 2026-08-06
Scope: architectures, industry integration, benchmark evidence, latency/cost, evaluation methodology.
Method: every factual claim is cited inline to a primary source (paper, official docs, first-party model card). Claims that could not be verified against a primary source are omitted or explicitly marked. All URLs were fetched and verified during this session unless noted.

---

## 1. Executive summary

- Reranking is a second-stage scoring step: a cheaper first stage retrieves candidates (e.g. top-50–100), a more expensive model re-scores only those candidates. The pattern is documented by BAAI ([bge-reranker-base card](https://huggingface.co/BAAI/bge-reranker-base): "use bge embedding model to retrieve top 100 relevant documents, and then use bge reranker to re-rank the top 100 document to get the final top-3 results"), Azure ([semantic-search-overview](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview): "only the top 50 results progress to semantic ranking"), and Weaviate ([reranking docs](https://weaviate.io/developers/weaviate/concepts/reranking)).
- Two architecture families dominate: **cross-encoders** (query+document joint encoding; strongest quality, highest cost per document) and **LLM rerankers** (listwise/pairwise prompting; quality near or above supervised models, cost dominated by tokens and latency) — plus **fusion** (RRF and variants) that combines retrievers before any rerank.
- On BEIR, cross-encoder reranking of BM25 top-100 outperformed BM25 on 16/18 datasets (+11% average relative nDCG@10) at "high computational costs" ([BEIR paper](https://arxiv.org/abs/2104.08663)). Rerankers cannot recover recall — they only reorder what retrieval found.
- Zero-shot LLM reranking is competitive with supervised cross-encoders: ChatGPT listwise reranking averaged 49.37 nDCG@10 on BEIR vs monoBERT 47.16; GPT-4 53.68; a permutation-distilled 440M DeBERTa reached 53.03, beating the 3B monoT5 supervised model ([RankGPT paper](https://arxiv.org/abs/2304.09542)). Pairwise prompting (PRP) with a 20B open model matched GPT-4 at ~50× smaller size ([PRP paper](https://arxiv.org/abs/2306.17563)).
- Industry pipelines that document their reranking are Azure AI Search semantic ranker (top-50, 2,048-token summaries, L2 scoring) and Cohere Rerank API (search-unit billed cross-encoder, ≤10,000 docs, auto-chunking). OpenAI Deep Research, Anthropic web search, and Perplexity do **not** publicly document their retrieval/reranking internals; only what their docs/announcements state is reported here.
- For ARC: a reranker must be evaluated as a delta vs the committed baseline per mode under ADR-0027 discipline (gold set, Corpus Fingerprint, 5% regression tolerance, retrieval/generation separation); key risk is that rerankers improve nDCG@10 but cannot fix recall@N, and that latency/cost must be measured on the real corpus, not benchmark corpora.

## 2. Architecture comparison

| Approach | Scoring | Latency profile | Cost profile | Benchmark evidence | Production maturity |
|---|---|---|---|---|---|
| **Cross-encoder** (monoBERT, BGE-reranker, Cohere Rerank, Azure semantic ranker) | Query + document concatenated, full cross-attention, single relevance score ([monoBERT paper](https://arxiv.org/abs/1901.04085); [Vespa cross-encoder docs](https://docs.vespa.ai/en/ranking/cross-encoders.html): "full cross-attention between all the query and document tokens") | Slow per document; must be applied only to a small candidate set; BEIR measured 450 ms GPU / 6,100 ms CPU per query reranking 1M-candidate doc set with BM25+CE ([BEIR paper](https://arxiv.org/abs/2104.08663), Table 3) | GPU inference per (query, doc) pair; index remains small (0.4 GB in BEIR's measurement); no doc-vector precompute | BEIR: BM25+CE best zero-shot model, +11% avg relative nDCG@10 over BM25, wins 16/18 datasets ([BEIR paper](https://arxiv.org/abs/2104.08663)); monoBERT: +27% relative MRR@10 over previous SOTA on MS MARCO leaderboard ([monoBERT paper](https://arxiv.org/abs/1901.04085)) | Highest — hosted APIs (Azure, Cohere) and self-hosted (BGE, Vespa ONNX) |
| **LLM rerankers** (listwise: RankGPT; pairwise: PRP) | Prompt the LLM to generate a permutation of candidates (listwise, sliding window) or compare pairs (pairwise); zero-shot ([RankGPT paper](https://arxiv.org/abs/2304.09542); [PRP paper](https://arxiv.org/abs/2306.17563)) | Seconds-to-minutes per query (generation), plus token latency; RankGPT paper re-reranks only top-20–30 with GPT-4 to control cost ([RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 footnote) | Token cost per candidate set; listwise prompts contain all candidates; pairwise is O(n²) naive, but linear-complexity variants exist ([PRP paper](https://arxiv.org/abs/2306.17563)) | BEIR avg nDCG@10: gpt-3.5-turbo 49.37 vs monoBERT 47.16; gpt-4 53.68 vs monoT5(3B) 51.36 ([RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1); PRP Flan-UL2-20B matches GPT-4 on TREC-DL 19/20, beats InstructGPT-175B by >10% ([PRP paper](https://arxiv.org/abs/2306.17563)) | Emerging; used for eval/pilot, rarely for per-query rerank in production due to cost — no production vendor documents it; Anthropic's dynamic filtering is the closest production analogue (LLM-driven result filtering, [web search tool docs](https://docs.claude.com/en/docs/agents-and-tools/tool-use/web-search-tool)) |
| **Hybrid / fusion** (RRF, weighted RRF, DBSF) | Rank-based or score-distribution fusion of multiple retrievers (e.g. dense + sparse) — no learned model ([Cormack et al. 2009](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf); [Qdrant hybrid docs](https://qdrant.tech/documentation/search/hybrid-queries/)) | Microseconds-scale (pure arithmetic on retrieved ranks); adds negligible latency vs retrieval itself | None beyond retrieval | Qdrant: "RRF (the safe default)" when no eval set; weighted RRF tuned on a train/val split when an eval set exists ([Qdrant hybrid docs](https://qdrant.tech/documentation/search/hybrid-queries/)); Azure feeds RRF results into its semantic ranker ([Azure semantic overview](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview)) | Very high — native in Qdrant, Weaviate, Azure (RRF hybrid scoring); often combined with a reranker on top |

Note on IDs: the task brief referenced arXiv:2306.17542 and arXiv:1904.04923 for RankGPT and monoBERT. Both IDs were verified to be **wrong** (2306.17542 is a physics/optics paper "[Multipass wide-field phase imager](https://arxiv.org/abs/2306.17542)"; 1904.04923 is an astrophysics paper). Correct IDs: RankGPT = [arXiv:2304.09542](https://arxiv.org/abs/2304.09542), monoBERT = [arXiv:1901.04085](https://arxiv.org/abs/1901.04085).

### 2.1 Cross-encoder rerankers

- Approach: the query and one document are fed together into a transformer with full cross-attention; the output is a single relevance score ([Vespa cross-encoder docs](https://docs.vespa.ai/en/ranking/cross-encoders.html)). monoBERT pioneered this for passage reranking and was "the top entry in the leaderboard of the MS MARCO passage retrieval task, outperforming the previous state of the art by 27% (relative) in MRR@10" ([monoBERT paper](https://arxiv.org/abs/1901.04085)).
- BGE rerankers: "Different from embedding model, reranker uses question and document as input and directly output similarity instead of embedding" ([bge-reranker-v2-m3 card](https://huggingface.co/BAAI/bge-reranker-v2-m3)); the family is described as "a cross-encoder model which is more accurate but less efficient" than bi-encoders ([bge-reranker-base card](https://huggingface.co/BAAI/bge-reranker-base)). Scores are unbounded logits; the card recommends sigmoid normalization for [0,1] mapping ([bge-reranker-v2-m3 card](https://huggingface.co/BAAI/bge-reranker-v2-m3)).
- Scoring semantics: scores are query-dependent and only meaningfully comparable within one query's candidate set; Cohere explicitly warns "you can't assume that a document with a relevance score of 0.9109375 is twice as relevant as one with a relevance score of 0.04421997" ([Cohere best practices](https://docs.cohere.com/docs/reranking-best-practices)).
- Strengths: best quality per pair (BEIR's best zero-shot system was BM25+CE, [BEIR paper](https://arxiv.org/abs/2104.08663)); works on text only and cannot re-search the corpus (Azure: "Semantic ranking reranks the existing result set... it can't rerun the query over the entire corpus", [Azure docs](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview)). Weakness: cost scales linearly with the number of candidates.

### 2.2 LLM rerankers

- Listwise (RankGPT): instruct the LLM to output a permutation of the candidate passages, applied over a sliding window. On BEIR (all models reranking the same BM25 top-100): BM25 43.42, monoBERT 47.16, monoT5(3B) 51.36, Cohere Rerank-v2 49.45, gpt-3.5-turbo 49.37, gpt-4 53.68 (GPT-4 reranked top-30 of GPT-3.5's output "to reduce the cost of calling gpt-4 API") ([RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1).
- Pairwise (PRP): "significantly reduce the burden on LLMs" by asking which of two documents is better; "results are the first in the literature to achieve state-of-the-art ranking performance on standard benchmarks using moderate-sized open-sourced LLMs"; Flan-UL2 (20B) "performs favorably with the previous best approach in the literature, which is based on the blackbox commercial GPT-4 that has 50x (estimated) model size"; on seven BEIR tasks PRP "outperforms the blackbox commercial ChatGPT solution by 4.2% and pointwise LLM-based solutions by more than 10% on average NDCG@10"; efficiency variants reach "competitive results even with linear complexity" ([PRP paper](https://arxiv.org/abs/2306.17563)).
- Sensitivity: RankGPT found performance "highly sensitive to the initial passage order" (random order nDCG@10 25.17 vs BM25 order 65.80 on DL19) — the reranker is only as good as its candidate ordering allows ([RankGPT paper](https://arxiv.org/abs/2304.09542), Table 5).
- Production use: LLM reranking is cost-heavy; the RankGPT paper's own production-motivated design is distillation into a small specialized cross-encoder ("a distilled 440M model outperforms a 3B supervised model on the BEIR benchmark", [RankGPT paper](https://arxiv.org/abs/2304.09542)). No vendor documents running LLM rerankers per query in production; the closest documented production analogue is Anthropic's web search "dynamic filtering", where code filters search results "before they reach the context window... reduces token use on search-heavy requests" ([Anthropic web search docs](https://docs.claude.com/en/docs/agents-and-tools/tool-use/web-search-tool)).

### 2.3 Hybrid / fused reranking

- RRF: fuse ranked lists by `score(d) = Σ 1/(k + rank(d))` per source ranking; introduced in [Cormack et al., SIGIR 2009](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf) (PDF verified; Qdrant cites the same source and defaults `k=2`, [Qdrant hybrid docs](https://qdrant.tech/documentation/search/hybrid-queries/)).
- Weighted RRF (Qdrant ≥1.17) and DBSF (≥1.11, distribution-normalized score fusion) are documented alternatives; Qdrant's guidance: weighted RRF tuned on a train/val split if you have an eval set, DBSF if you trust raw scores and have no eval set, "RRF (the safe default)" otherwise; and that a fixed alpha on raw dense+sparse scores "is unreliable" because the two scales differ per query ([Qdrant hybrid docs](https://qdrant.tech/documentation/search/hybrid-queries/)).
- Rerankers-on-top: Azure's semantic ranker operates over "BM25-ranked result... or RRF-ranked result" and only the top 50 progress to semantic ranking ([Azure semantic overview](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview)).

## 3. Industry integration findings

### 3.1 Azure AI Search — semantic ranker

Documented (all from [semantic-search-overview](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview), [semantic billing](https://learn.microsoft.com/en-us/azure/search/semantic-how-to-enable-disable), [limits/quotas](https://learn.microsoft.com/en-us/azure/search/search-limits-quotas-capacity)):

- "L2 ranking": secondary ranking over an initial BM25- or RRF-scored result set, using "multilingual, deep learning models adapted from Microsoft Bing".
- Only the top 50 results progress to semantic ranking; each result is summarized to a maximum of 2,048 tokens (title ≤128, keywords ≤128, content remainder) before scoring.
- Output: `@search.rerankerScore` in [0,4] with documented score semantics (4 = complete answer... 0 = irrelevant); captions and optional answers are always verbatim text.
- Limitations: reranks the existing top-50 only; cannot re-search the corpus; best on "information-rich and structured as prose" content.
- Billing: premium feature; free plan (default) with a "monthly free request allowance" after which requests "return a billing error"; standard pay-as-you-go plan requires Basic tier or higher. The current docs do not state the exact free allowance number (older "queries per day" figures are not present in the pages verified here).
- Latency engineering: "Semantic ranking uses a lot of resources and time... the system consolidates and reduces inputs"; throttling limits are documented per search unit (max concurrent semantic requests: Basic 2, S1 3, S2–S3 4; with queue sizes 4/6/8), and the docs note query time "varies based on how busy the search service is".
- Relevance: does not rerank the corpus; gains are measurable only where recall@50 is already adequate.

### 3.2 Cohere Rerank API

Documented (from [Rerank overview](https://docs.cohere.com/docs/rerank-overview), [Rerank model details](https://docs.cohere.com/docs/rerank), [best practices](https://docs.cohere.com/docs/reranking-best-practices)):

- Endpoint semantics: given a query and list of documents, return documents ordered by semantic relevance with `relevance_score` normalized to [0,1].
- Models: `rerank-v4.0-pro`, `rerank-v4.0-fast` (multilingual, 32,768-token context), `rerank-v3.5` (4,096-token context), `rerank-english-v3.0` / `rerank-multilingual-v3.0`. Usage pattern: retrieve with an existing search solution, then rerank ("often used to sort search results returned from an existing search solution").
- Limits: up to 10,000 documents per request; query truncated at 16,384 tokens (v4.0) or 2,048 (v3.5); documents longer than the window are auto-chunked and scored with `max()` over chunk scores.
- Billing: responses bill `search_units` (seen in the API example response in the [overview](https://docs.cohere.com/docs/rerank-overview)); token usage is not separately itemized for rerank calls in the example response.
- Scoring caveat: scores are query-dependent; docs recommend calibrating a relevance threshold on 30–50 representative domain queries with borderline-relevant pairs ([best practices](https://docs.cohere.com/docs/reranking-best-practices)).
- BEIR evidence: Cohere Rerank-v2 averaged 49.45 nDCG@10 on BEIR (reported in the [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 — not in Cohere's own docs verified here).

### 3.3 OpenAI Deep Research / Anthropic Claude web search / Perplexity

What is actually documented:

- **OpenAI Deep Research**: the [announcement](https://openai.com/index/introducing-deep-research/) documents: an agentic, multi-step research loop powered by an o3 variant "optimized for web browsing"; tasks take 5–30 minutes; it "finds, analyzes, and synthesizes hundreds of online sources"; outputs fully cited reports; trained with reinforcement learning on browsing tasks; usage quotas per plan (Pro 100/month at launch, later 25/250 per month for Plus/Pro, [April 24, 2025 update](https://openai.com/index/introducing-deep-research/)). The [Deep Research System Card](https://cdn.openai.com/deep-research-system-card.pdf) documents safety mitigations — prompt-injection defenses, system-level blocklists, a constraint that the model "do[es] not allow deep research to navigate to or construct arbitrary URLs" — and states the model is "optimized for web browsing".
- **Not documented**: any retrieval/reranking architecture — number of search results per step, whether results are reranked, scorer type, context assembly. Speculation would be required; none is included here.
- **Anthropic Claude with search**: the [Claude web search announcement](https://www.anthropic.com/news/web-search) (March 20, 2025; globally available May 27, 2025) documents: Claude decides when to search, provides direct citations for fact-checking. The [web search tool docs](https://docs.claude.com/en/docs/agents-and-tools/tool-use/web-search-tool) document: three tool versions (`web_search_20250305` basic; `web_search_20260209` adds "dynamic filtering"; `web_search_20260318` adds response-inclusion control); dynamic filtering — "every search result is loaded into Claude's context window" with basic search, whereas later versions filter results with code before they reach the context window, "reducing token use on search-heavy requests"; citations are always enabled with `cited_text` (≤150 chars); search counts ("simple factual queries typically use 1–3 searches; comparative or multientity research can use 10 or more"); `max_uses`, `allowed_domains`/`blocked_domains`, `user_location`; pricing "**$10 per 1,000 searches**, plus standard token costs for search-generated content".
- **Not documented**: the search backend (retriever, candidate count, reranking of search results) is not disclosed.
- **Perplexity**: the [docs](https://docs.perplexity.ai/) document the Search API ("raw, ranked web search results with advanced filtering") and Agent API ("web-grounded answers with built-in citations"), plus `search_domain_filter` / `search_recency_filter` controls on the web_search tool.
- **Not documented**: any reranking component or candidate-selection details.

### 3.4 Vector DB integrations

- **Qdrant**: `prefetch` (nested sub-queries) + main query enables hybrid fusion and multi-stage rescoring natively: RRF with tunable `k`, weighted RRF (v1.17+), DBSF (v1.11+), and "multi-stage queries" — e.g. MRL byte-vector prefetch (1,000 candidates) → full-vector rescore → top-10, with HNSW disabled (`m=0`) for rescoring-only vectors to save memory ([hybrid queries docs](https://qdrant.tech/documentation/search/hybrid-queries/)).
- **Weaviate**: reranker modules (Cohere, transformers, VoyageAI) applied as a second stage after vector, BM25, or hybrid search; "Computing this score for all (query, data_object) pairs would typically be prohibitively slow, which is why reranking is used as a second stage" ([reranking docs](https://weaviate.io/developers/weaviate/concepts/reranking)). The docs link the [multi-stage search design](https://weaviate.io/blog/cross-encoders-as-reranker).
- **Vespa**: phased ranking with cross-encoder ONNX models (e.g. `intfloat/simlm-msmarco-reranker`, `BAAI/bge-reranker-base`); "Cross-Encoder Transformer based text ranking models are generally more effective than text embedding models as they take both the query and the document as input with full cross-attention... The downside of cross-encoder models is the computational complexity" ([cross-encoder ranking docs](https://docs.vespa.ai/en/ranking/cross-encoders.html); phased ranking documented at [docs.vespa.ai/en/ranking/phased-ranking.html](https://docs.vespa.ai/en/ranking/phased-ranking.html)).
- **Milvus**: Milvus maintains a rerank documentation page ([milvus.io/docs/rerank.md](https://milvus.io/docs/rerank.md)) but it was unreachable (timeout/403) from this environment across multiple attempts, so no claims about its contents are made here.

## 4. Benchmark evidence table

All values as reported in the cited primary sources. BEIR = average nDCG@10 across BEIR datasets unless noted; all LLM-rerank rows rerank the same BM25 top-100 unless noted.

| Model / system | Dataset | Metric | Value | Source |
|---|---|---|---|---|
| BM25 (baseline) | BEIR (avg) | nDCG@10 | 0.4342 | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 |
| BM25 + cross-encoder (MiniLM, top-100) | BEIR (avg) | nDCG@10 (rel. vs BM25) | +11% avg; best on 16/18 datasets | [BEIR paper](https://arxiv.org/abs/2104.08663), Table 2 |
| monoBERT (340M) | BEIR (avg) | nDCG@10 | 0.4716 | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 |
| monoT5 (3B) | BEIR (avg) | nDCG@10 | 0.5136 | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 |
| Cohere Rerank-v2 | BEIR (avg) | nDCG@10 | 0.4945 | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 |
| gpt-3.5-turbo listwise (RankGPT) | BEIR (avg) | nDCG@10 | 0.4937 | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 |
| gpt-4 listwise (over gpt-3.5 top-30) | BEIR (avg) | nDCG@10 | 0.5368 | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 1 |
| Permutation-distilled DeBERTa-Large | BEIR (avg, 8 datasets) | nDCG@10 | 0.5303 (beats monoT5-3B 0.5136) | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 7 |
| monoBERT | MS MARCO dev leaderboard | MRR@10 | +27% relative over previous SOTA | [monoBERT paper](https://arxiv.org/abs/1901.04085) |
| PRP (Flan-UL2 20B, pairwise) | TREC-DL 19/20 | nDCG@10 | "performs favorably with" GPT-4 (50× larger); +10%+ over InstructGPT-175B | [PRP paper](https://arxiv.org/abs/2306.17563) |
| PRP (Flan-UL2 20B, pairwise) | 7 BEIR tasks | avg NDCG@10 | +4.2% over ChatGPT; +10%+ over pointwise LLM baselines | [PRP paper](https://arxiv.org/abs/2306.17563) |
| gpt-4 | NovelEval (unseen knowledge) | nDCG@10 | 0.9045 (vs monoBERT 0.7727) | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 3 |
| gpt-3.5-turbo (BM25 order vs random order) | TREC-DL19 | nDCG@10 | 0.6580 vs 0.2517 — order sensitivity | [RankGPT paper](https://arxiv.org/abs/2304.09542), Table 5 |
| bge-reranker-base / large | C-MTEB reranking avg | MAP | 65.42 / 66.09 (vs bge-large-zh-v1.5 embedding 63.97) | [bge-reranker-base card](https://huggingface.co/BAAI/bge-reranker-base) |

Notes on scope differences:

- **Retrieval vs reranking benchmarks**: BEIR and the MTEB retrieval/reranking tasks measure candidate ranking quality (nDCG@10, recall@100), not end-to-end RAG answer quality. MTEB spans 8 embedding tasks / 58 datasets / 112 languages and includes a dedicated reranking task cluster ([MTEB paper](https://arxiv.org/abs/2210.07316)); the live leaderboard is at [huggingface.co/spaces/mteb/leaderboard](https://huggingface.co/spaces/mteb/leaderboard). An nDCG@10 gain from a reranker says nothing directly about answer faithfulness, citation accuracy, or abstraction quality.
- **Annotation bias**: BEIR's TREC-COVID analysis shows rerankers/dense models retrieve unjudged top hits (Hole@10: BM25+CE 1.6% vs TAS-B 31.8%), so benchmark deltas on pooled datasets can be distorted — an argument for corpus-grounded, fully judged evaluation like ARC's Gold Set ([BEIR paper](https://arxiv.org/abs/2104.08663), Table 4).

## 5. Latency / cost

### 5.1 Measured numbers from primary sources

| System | Latency | Index / cost | Source |
|---|---|---|---|
| BM25+CE rerank (1M-doc corpus, reranking top-100) | 450 ms GPU / 6,100 ms CPU per query | 0.4 GB | [BEIR paper](https://arxiv.org/abs/2104.08663), Table 3 |
| Dense retrievers (TAS-B, ANCE, DPR) | 14–20 ms GPU / 125–275 ms CPU | 3 GB | [BEIR paper](https://arxiv.org/abs/2104.08663), Table 3 |
| Azure semantic ranker | "uses a lot of resources and time"; inputs consolidated to ≤2,048 tokens/doc; concurrency limited (Basic 2, S1 3, S2–S3 4 concurrent/SU, queue beyond) | billed per request beyond free monthly allowance | [Azure docs](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview), [limits](https://learn.microsoft.com/en-us/azure/search/search-limits-quotas-capacity) |
| Anthropic web search | n/a (per-search latency not documented); 1–3 searches typical factual, 10+ comparative | **$10 per 1,000 searches** + token cost of retrieved content (search results counted as input tokens) | [Anthropic web search docs](https://docs.claude.com/en/docs/agents-and-tools/tool-use/web-search-tool) |

### 5.2 LLM reranker cost characteristics

- Token cost scales with candidate count and prompt style: listwise prompts pack all candidates into one (or a sliding window of) prompts; the RankGPT paper's cost control is to rerank top-30 with GPT-4 over gpt-3.5 output (Table 1 footnote, [arXiv:2304.09542](https://arxiv.org/abs/2304.09542)) and its practical recommendation is distillation to a small cross-encoder ("a distilled 440M model outperforms a 3B supervised model on BEIR", [abstract](https://arxiv.org/abs/2304.09542)).
- Pairwise is O(n²) naive; the PRP paper documents efficiency variants achieving "competitive results even with linear complexity" ([PRP paper](https://arxiv.org/abs/2306.17563)).
- Typical candidate sizes in the literature: 20 (PRP top-20 on DL19, [arXiv:2306.17563](https://arxiv.org/abs/2306.17563)), 20–100 (RankGPT uses top-20 with sliding windows and top-100 for the initial pool, [arXiv:2304.09542](https://arxiv.org/abs/2304.09542)); supervised rerankers on BEIR rerank "top-100 retrieved hits" ([BEIR paper](https://arxiv.org/abs/2104.08663)).

### 5.3 The production pattern: retrieve N → rerank → keep K

Documented instances (primary sources):

- BGE: retrieve top-100 with an embedding model, rerank, keep top-3 ([bge-reranker-base card](https://huggingface.co/BAAI/bge-reranker-base)).
- Azure: top-50 reranked by the semantic ranker; response includes rescored results with captions/answers ([Azure docs](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview)).
- Cohere: rerank over documents retrieved by an existing search solution, with `top_n` controlling how many of the reranked results to return ([Cohere overview](https://docs.cohere.com/docs/rerank-overview)); up to 10,000 documents per call ([best practices](https://docs.cohere.com/docs/reranking-best-practices)).
- Qdrant: prefetch limit for dense/sparse (e.g. 20 or 100) → fusion/rerank → final limit (e.g. 10) ([hybrid queries docs](https://qdrant.tech/documentation/search/hybrid-queries/), [reranking tutorial](https://qdrant.tech/documentation/tutorials-basics/reranking-hybrid-search/)).
- Vespa: phased ranking — "cheap ranking phases first, more expensive phases later" over a limited hit set ([phased ranking docs](https://docs.vespa.ai/en/ranking/phased-ranking.html)).

Implication for ARC: a reranker stage is only as good as the recall of the first stage (top-N), and the cost per query is (first-stage latency) + (N × cross-encoder latency) or (LLM tokens). Reranking cannot recover chunks the first stage never returned — the RankGPT limitation statement is explicit: "the upper bound of the ranking effect is contingent upon the recall of the initial passage retrieval" ([RankGPT paper](https://arxiv.org/abs/2304.09542), Limitations).

## 6. Evaluation methodology (aligned with ARC's benchmark discipline)

ARC's existing harness is defined in [docs/benchmarks/README.md](../benchmarks/README.md) and [ADR-0027](../adr/0027-retrieval-evaluation-methodology.md): versioned Gold Set of ~50 queries across six intent categories, `expected_chunk_ids`/`expected_sections`/`expected_no_evidence`, Corpus Fingerprint hard-fail, retrieval/generation separation, calibration-first regression gates with a 5% tolerance vs committed baselines and floors at baseline − 5 points, and reports recording "reranker (if any)" among their reproducibility fields. A reranker research/eval design must fit that discipline:

1. **Measure before and after, on the same Gold Set**: report `Recall@N` for the first stage (unchanged by reranking — a reranker reorders, it cannot add chunks), then `nDCG@K`, `MRR@K`, `Precision@K` after reranking at the same K as the committed baseline (e.g. nDCG@5 vs the committed 0.645 dense / 0.668 hybrid baselines, [benchmarks README](../benchmarks/README.md)). If recall@5 with `RETRIEVAL_MIN_SCORE=0.6` is 0.740, a reranker's ceiling on the same candidates is bounded by that recall.
2. **Rerankers change the score distribution**: BGE scores are unbounded logits ([bge-reranker-v2-m3 card](https://huggingface.co/BAAI/bge-reranker-v2-m3)); Cohere explicitly warns scores are query-dependent and recommends calibrating any threshold on 30–50 domain queries ([Cohere best practices](https://docs.cohere.com/docs/reranking-best-practices)). ARC's calibrated `RETRIEVAL_MIN_SCORE` gate and abstention precision must be re-calibrated per reranker, per mode — never reused from the bi-encoder calibration. The M3 finding that "a single global cosine threshold cannot simultaneously maximize recall and abstention precision" ([benchmarks README](../benchmarks/README.md)) should be re-tested per reranker.
3. **Per-category deltas, not just averages**: ADR-0027 gates every retrieval change on per-category metrics; Gold Set categories (`comparison`, `entity`, `abstention`, ...) are exactly where rerankers can hurt (e.g. entity queries already handled by hybrid sparse in M3). A reranker that lifts nDCG@5 but regresses abstention `no_evidence_precision` fails the gate.
4. **Latency/cost is part of the benchmark**: report rerank latency and (if LLM-based) token cost per query in the manifest, mirroring the duration reporting already in [benchmarks README](../benchmarks/README.md) (dense ~10.8s vs sparse ~1.4s for 51 queries). Cross-encoder latency on ARC's corpus (~200 chunks) is small, but candidate-set size (top-N fed to the reranker) and model choice dominate; the BEIR measurements (450 ms GPU/6,100 ms CPU per query for BM25+CE over a 1M corpus, [BEIR paper](https://arxiv.org/abs/2104.08663)) are the only directly sourced reference points — ARC should measure its own.
5. **Do not tune on the Gold Set**: ADR-0027's calibration-first rule ("thresholds are never invented before measurement") extends to rerankers — the Gold Set is the test set; any reranker selection (model, top-N, threshold, fusion weights) must be decided before running the gate or on a held-out split. Qdrant's own docs make the same point: "Measuring on the same queries you tuned on inflates the result... split your eval queries in two. Try different weights on the first half, then measure on the second half" ([Qdrant hybrid docs](https://qdrant.tech/documentation/search/hybrid-queries/)).
6. **Beware benchmark-vs-production gap**: BEIR's Hole@10 analysis shows pooled benchmarks under-judge non-lexical retrievers ([BEIR paper](https://arxiv.org/abs/2104.08663)); ARC's fingerprint-gated, fully-judged Gold Set avoids this. Also note rerankers trained on one domain can regress on another (BEIR: BM25+CE "fails on ArguAna and Touché-2020, two retrieval tasks extremely different to the MS MARCO training dataset", [BEIR paper](https://arxiv.org/abs/2104.08663)) — domain fit must be validated on ARC's own corpus.
7. **End-to-end metrics stay a separate layer**: per ADR-0027, generation metrics (faithfulness, citation accuracy, completeness, `no_evidence` behavior) are a second evaluation layer on top of retrieval. A reranker should be judged first on the retrieval layer; only then on end-to-end answer quality. Note that ARC's `VerificationStatus`/abstention machinery ([CONTEXT.md](../CONTEXT.md) glossary) interacts with rerankers through scores and ordering, so both layers should be re-run.
8. **Registration**: ADR-0027 reports already include a `reranker (if any)` field; new baselines for a reranked mode must be committed with a full manifest before tuning, per the README's "New baselines" rule.

## 7. Sources

### Papers

- Passage Re-ranking with BERT (monoBERT): Nogueira & Cho — https://arxiv.org/abs/1901.04085
- BEIR: Thakur et al., NeurIPS 2021 — https://arxiv.org/abs/2104.08663
- MTEB: Muennighoff et al. — https://arxiv.org/abs/2210.07316 (leaderboard: https://huggingface.co/spaces/mteb/leaderboard)
- Is ChatGPT Good at Search? (RankGPT): Sun et al., EMNLP 2023 — https://arxiv.org/abs/2304.09542
- Pairwise Ranking Prompting (PRP): Qin et al., NAACL 2024 — https://arxiv.org/abs/2306.17563
- Reciprocal Rank Fusion: Cormack, Clarke, Buettcher, SIGIR 2009 — https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf
- (Corrected citation notes: arXiv:2306.17542 = "Multipass wide-field phase imager" (physics), NOT the RankGPT paper; arXiv:1904.04923 = EHT GRMHD code comparison (astrophysics), NOT monoBERT.)

### Model cards / open-source

- BAAI/bge-reranker-v2-m3 — https://huggingface.co/BAAI/bge-reranker-v2-m3
- BAAI/bge-reranker-base — https://huggingface.co/BAAI/bge-reranker-base
- Vespa cross-encoder ranking guide — https://docs.vespa.ai/en/ranking/cross-encoders.html
- Vespa phased ranking — https://docs.vespa.ai/en/ranking/phased-ranking.html

### Industry docs

- Azure AI Search semantic ranking overview — https://learn.microsoft.com/en-us/azure/search/semantic-search-overview
- Azure semantic ranker billing — https://learn.microsoft.com/en-us/azure/search/semantic-how-to-enable-disable
- Azure AI Search limits & quotas (semantic ranker throttling) — https://learn.microsoft.com/en-us/azure/search/search-limits-quotas-capacity
- Cohere Rerank overview — https://docs.cohere.com/docs/rerank-overview
- Cohere Rerank model details — https://docs.cohere.com/docs/rerank
- Cohere Rerank best practices — https://docs.cohere.com/docs/reranking-best-practices
- OpenAI Deep Research announcement — https://openai.com/index/introducing-deep-research/
- OpenAI Deep Research System Card — https://cdn.openai.com/deep-research-system-card.pdf
- Anthropic web search announcement — https://www.anthropic.com/news/web-search (also https://claude.com/blog/web-search)
- Anthropic web search tool docs — https://docs.claude.com/en/docs/agents-and-tools/tool-use/web-search-tool
- Perplexity API docs — https://docs.perplexity.ai/
- Qdrant hybrid queries (RRF, weighted RRF, DBSF, multi-stage) — https://qdrant.tech/documentation/search/hybrid-queries/
- Qdrant hybrid search with reranking tutorial — https://qdrant.tech/documentation/tutorials-basics/reranking-hybrid-search/
- Weaviate reranking — https://weaviate.io/developers/weaviate/concepts/reranking
- Milvus rerank docs — https://milvus.io/docs/rerank.md (unreachable from this environment; cited as intended primary source only)

### ARC repo references (aligned terminology, unmodified)

- Repo-local (read-only, terminology alignment only): [docs/benchmarks/README.md](../benchmarks/README.md), [docs/adr/0027-retrieval-evaluation-methodology.md](../adr/0027-retrieval-evaluation-methodology.md).

## 8. Unresolved questions

Primary sources do not answer these; listed for follow-up rather than filled with speculation:

1. **Vendor reranking internals**: OpenAI Deep Research, Anthropic web search, and Perplexity do not disclose candidate counts, scorer models, or whether results are reranked at all. Anthropic's "dynamic filtering" ([docs](https://docs.claude.com/en/docs/agents-and-tools/tool-use/web-search-tool)) is the only documented retrieval-side optimization, and its mechanism (LLM-written filter code) is not characterized in retrieval-metric terms.
2. **Azure semantic ranker specifics**: the exact monthly free allowance, the underlying model identity ("adapted from Microsoft Bing" is all the [overview](https://learn.microsoft.com/en-us/azure/search/semantic-search-overview) states), and whether the top-50 cap is configurable are not documented in the pages verified.
3. **Cohere rerank model architecture and latency**: model cards/docs do not state parameters, per-query latency, or the BEIR score of current models (rerank-v3.5/v4.0); the only officially reported BEIR figure found is for Rerank-v2 in the RankGPT paper ([Table 1](https://arxiv.org/abs/2304.09542)).
4. **BGE-reranker-v2-m3 exact BEIR numbers**: the model card's evaluation section renders results as images ("It rereank the top 100 results from bge-en-v1.5 large", [card](https://huggingface.co/BAAI/bge-reranker-v2-m3)); the numeric values could not be verified from the card text and are therefore omitted.
5. **Cross-encoder per-document latency**: no primary source gives a clean per-document ms figure for representative models on CPU/GPU (BEIR's 450/6,100 ms figures are per-query over a rerank pipeline, [Table 3](https://arxiv.org/abs/2104.08663)). Any ARC latency budget must be measured locally.
6. **Milvus rerank documentation**: page unreachable from this environment; its contents could not be verified.
7. **Production use of LLM rerankers**: no vendor documents using LLM-based listwise/pairwise reranking in a per-query production path; the only production-ish evidence is distillation to small cross-encoders ([RankGPT paper](https://arxiv.org/abs/2304.09542)).
8. **ColPali operational cost at scale**: the paper claims simplicity and speed ([arXiv:2407.01449](https://arxiv.org/abs/2407.01449)) but no primary source found states per-page indexing cost or storage overhead on a production corpus; ViDoRe benchmark details are in the paper, not re-summarized here.
