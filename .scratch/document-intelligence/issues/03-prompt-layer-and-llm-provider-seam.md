# 03 — Prompt Layer & LLM Provider Seam

**What to build:**
The vendor-agnostic `PromptBuilder` in `internal/qa/prompt`, system instructions, `PromptMessage` domain models, `LLMProvider` interface seam (`Generate`, `Stream`, `Capabilities`) in `internal/llm/provider`, and `MockLLMProvider` adapter.

**Blocked by:** 01 — QA Orchestration Core.

**Status:** ready-for-agent

- [ ] Define `PromptMessage`, `GenerationOptions`, and `ModelCapabilities` models.
- [ ] Implement `PromptBuilder` interface and `RAGPromptBuilder` constructing system prompts and RAG citation constraints.
- [ ] Define `LLMProvider` interface seam and `MockLLMProvider` generating deterministic responses with `[Ref N]` markers.
- [ ] Add unit tests verifying prompt assembly, vendor isolation, and mock LLM token generation.
