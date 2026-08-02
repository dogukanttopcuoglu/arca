# 17 — SummaryExtractor Interface & Extractive RuleBasedSummaryExtractor

**What to build:**
Implement `SummaryExtractor` interface seam and extractive `RuleBasedSummaryExtractor` in `internal/pdfinspector/enrichment/summary_extractor.go`.

**Blocked by:** 16 — Summary Domain Model & Capability Token.

**Status:** ready-for-agent

- [ ] Define `SummaryInput`, `SummaryResult`, and `SummaryExtractor` interface seam (`ExtractSummaries(ctx, input) (SummaryResult, error)`).
- [ ] Implement extractive `RuleBasedSummaryExtractor` selecting top informative sentences, concepts, and entities.
- [ ] Add unit tests verifying empty input, document-level extractive summary, and chunk-level extractive summaries.
