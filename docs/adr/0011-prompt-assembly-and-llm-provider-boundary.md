# 0011: Prompt Assembly & LLM Provider Boundary

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

To support switching between LLM providers (OpenAI, Anthropic Claude, Ollama, Llama 3, Mock), system instructions, RAG citation rules, and prompt engineering must be isolated from vendor API transport logic.

## Decision Drivers

- **Vendor Lock-in Avoidance**: Prompts and system instructions belong to ARC domain, not LLM SDK adapters.
- **Scientific Consistency**: Evaluating different LLM models requires sending identical system instructions and citation guidelines.

## Decided Options

### Option B: Independent `internal/qa/prompt` Layer (ACCEPTED)
- `internal/qa/prompt` owns system instructions, citation rules, and `PromptBuilder` interface (`Build(query, context) PromptMessage`).
- `internal/llm/provider` owns the `LLMProvider` interface seam (`Generate(ctx, prompt) (*LLMResponse, error)`) and `ModelCapabilities` (`SupportsSystemMessage`, `SupportsStreaming`, `ContextWindow`).
- Concrete LLM adapters (`llm/openai`, `llm/anthropic`, `llm/ollama`, `llm/mock`) are vendor transport adapters only.

### Option A: Prompts Embedded in Provider Adapters (REJECTED)
Embedding system instructions and RAG citation rules inside vendor-specific provider packages.

## Consequences

### Positive
- Switching from OpenAI to Claude or Ollama requires zero prompt code changes.
- Vendor adapters remain narrow HTTP payload translators.
