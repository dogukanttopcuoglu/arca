# 01 — QA Orchestration Core & Modular Pipeline Composition

**What to build:**
The high-level `AnswerEngine` orchestrator in `internal/qa` implementing modular pipeline composition across `QueryAnalyzer`, `Retriever`, `ContextBuilder`, `PromptBuilder`, `LLMProvider`, and `CitationExtractor`.

**Blocked by:** Existing Indexing & Retrieval modules.

**Status:** ready-for-agent

- [ ] Define `AnswerEngine` struct and constructor.
- [ ] Define `QueryAnalyzer` interface seam (`Analyze(ctx, query string) (AnalyzedQuery, error)`).
- [ ] Implement `RuleBasedAnalyzer` default adapter for query intent extraction.
- [ ] Add unit tests verifying end-to-end sync orchestration flow with mock collaborators.
