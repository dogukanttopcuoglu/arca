# 0009: RAG Orchestration Pipeline & Modular Composition Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

Following the completion of `ADR-0008` (Knowledge Indexing & Retrieval Architecture), ARC requires an orchestration layer to answer user queries with verifiable citations. A monomorphic QA service creates tight coupling between Query Parsing, Retrieval, Prompt Construction, and LLM execution. We need a modular, deep-module orchestration pattern that avoids vendor lock-in and respects Clean Architecture.

## Decision Drivers

- **Clean Architecture & Deep Modules**: High-level orchestration must depend on narrow interface seams, not concrete implementations.
- **Graceful Degradation**: Optional stages (such as Result Evaluation or Reranking) must be pluggable without breaking the main execution path.
- **Vendor Independence**: Changing LLM providers or Query Analyzers must not require mutating core QA orchestration logic.

## Decided Options

### Option A: Modular Pipeline Composition (ACCEPTED)
`AnswerEngine` acts as a high-level composition orchestrator across narrow interface seams:
- `QueryAnalyzer`: Analyzes user query, extracts intent, entities, and filters.
- `Retriever`: Fetches relevant `KnowledgeChunk`s across Dense, Sparse, or Graph channels.
- `ResultEvaluator` (Optional): Evaluates retrieval confidence and reranks results.
- `ContextBuilder`: Assembles prompt-ready context window with token budgeting and citation keys.
- `PromptBuilder`: Formats system instructions and user context into a `PromptMessage`.
- `LLMProvider`: Generates text or streams tokens from LLMs.

### Option B: Monolithic QA Service (REJECTED)
A monolithic service with hardcoded dependencies on specific retriever types, prompt strings, and LLM client SDKs.

## Consequences

### Positive
- **Flexibility**: Individual pipeline stages can be tested, mocked, or swapped independently.
- **Resilience**: Optional evaluation steps do not block primary answer generation if unavailable.
- **Testability**: Every seam has isolated unit test coverage.

### Negative
- Requires distinct domain types (`AnalyzedQuery`, `ContextWindow`, `PromptMessage`, `AnswerDraft`).
