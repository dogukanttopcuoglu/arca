# 05 — RuleBasedKeywordExtractor & Stopword Filtering

**What to build:**
Implement `KeywordExtractor` interface seam and `RuleBasedKeywordExtractor` with TR/EN stopword filtering in `internal/pdfinspector/enrichment/keyword_extractor.go`.

**Blocked by:** 04 — Keyword Domain Model & LanguageDetectionPass.

**Status:** ready-for-agent

- [ ] Define `KeywordExtractor` interface seam (`Extract(ctx, chunks, lang) ([]Keyword, error)`).
- [ ] Implement `RuleBasedKeywordExtractor` calculating term-frequency scores and applying Turkish/English stopword filters.
- [ ] Support keyword score normalization, confidence scoring, and deduplication.
- [ ] Add comprehensive unit tests verifying empty docs, single chunk, multi-chunk merge, stopwords, unicode, Turkish & English.
