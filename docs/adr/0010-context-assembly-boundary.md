# 0010: Context Assembly Boundary & Token Budgeting

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

`SearchResult` objects produced by `Retriever` cannot be injected directly into LLM prompts without token budgeting, duplicate removal, section re-ordering, and citation marker injection. We must decide where `ContextBuilder` lives and whether LLM token limits leak into the Retrieval layer.

## Decision Drivers

- **Separation of Concerns**: `Retriever` finds relevant information; it does not format LLM prompts or count LLM tokens.
- **Citation Integrity**: Citation keys (`[Ref 1]`, `[Ref 2]`) must be assigned deterministically during context assembly.
- **Provider Agnosticism**: Context building must use a `TokenCounter` abstraction rather than hardcoding vendor tokenizers.

## Decided Options

### Option B: `internal/qa/context` (ACCEPTED)
`ContextBuilder` resides in `internal/qa/context`. It takes `[]SearchResult` and outputs a `ContextWindow` (`Sources`, `Content`, `TokenCount`). It uses sub-seams:
- `TokenCounter`: Counts tokens for a target model context budget.
- `CitationFormatter`: Injects immutable reference markers (`[Ref N]`) and formats source headers.
- `ReorderEngine`: Re-orders chunks logically by page number and section hierarchy.

### Option A: `internal/retrieval/context` (REJECTED)
Coupling LLM prompt assembly and token budget logic to the low-level `VectorStore` or `Retriever` packages.

## Consequences

### Positive
- `Retriever` remains a pure search abstraction.
- Token budgeting and citation formatting are encapsulated cleanly inside QA orchestration.

### Negative
- Requires maintaining `TokenCounter` adapters for different model tokenizers.
