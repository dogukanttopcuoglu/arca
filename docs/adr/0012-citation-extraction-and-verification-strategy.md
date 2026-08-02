# 0012: Citation Extraction & Verification Strategy

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

ARC's primary value proposition is providing verifiable, hallucination-free answers backed by PDF page numbers and chunk citations. We must decide how citations are extracted from LLM responses and verified against source documents.

## Decision Drivers

- **Graceful Degradation**: If an LLM response format degrades, the answer text must not be lost.
- **Model Interoperability**: Must work reliably across closed APIs (OpenAI, Claude) and open-weights local models (Ollama / Llama 3) without requiring native JSON Schema support.
- **Immutable References**: LLMs are not citation authorities; inline markers (`[Ref N]`) must be deterministically mapped back to verified `ContextWindow` sources.

## Decided Options

### Option A: Inline Markers + Post-Processing Extractor (ACCEPTED)
- System prompts instruct LLMs to embed natural inline reference markers (`[Ref 1]`, `[Ref 2]`).
- `internal/qa/citation` contains `CitationExtractor` and `VerificationReport`:
  - `Extract(answerText, contextWindow)` parses markers using AST/regex, maps them back to verified `ContextWindow.Sources`, checks page numbers and chunk IDs, and computes a `VerificationReport` (`TotalClaims`, `VerifiedClaims`, `InvalidReferences`).
  - Unmatched or hallucinated references (`[Ref 99]`) mark `IsVerified = false`.

### Option B: Mandatory LLM Structured JSON Output (REJECTED)
Requiring LLMs to output strict JSON schemas (`{ "answer": "...", "citations": [...] }`), which fails on smaller local models and breaks SSE streaming.

## Consequences

### Positive
- High reliability across all LLM providers and open-weights models.
- Supports real-time token streaming with finalization verification.
- Guarantees zero invented citations via immutable `ContextWindow` mapping.
