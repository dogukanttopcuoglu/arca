# 0024: Summary Extraction Architecture & Extractive Strategy Seam

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

Following `RelationExtractorPass` (ADR-0023), ARC requires summary extraction capabilities at both document and chunk levels to facilitate rapid LLM context compression and targeted retrieval. The summary architecture must support pluggable strategies (extractive vs. generative) without leaking orchestration logic into extractors.

## Decision Drivers

- **Minimal Summary Model**: Model `Summary` as a lean struct (`Text`, `Source`) attached to both `DocumentMetadata.Summary` and `KnowledgeChunk.Summary`.
- **Complete Semantic Input**: Provide `SummaryInput` with all previously extracted semantic artifacts (`Chunks`, `Keywords`, `Entities`, `Concepts`, `Relations`).
- **Explicit Producer-Orchestrator Seam**: `SummaryExtractor` acts as a pure producer returning `SummaryResult` (`DocumentSummary`, `ChunkSummaries`). `SummaryPass` owns all attachment logic.
- **Strictly Extractive MVP**: Initial `RuleBasedSummaryExtractor` uses deterministic extractive summarization (selecting informative sentences, top concepts, and entities) without abstractive prose synthesis.

## Decided Options

### Option A: Extractive Summary Strategy & Pure Producer Seam (ACCEPTED)

- **`Summary` & Enums**:
  ```go
  type SummarySource string

  const (
      SummarySourceRuleBased SummarySource = "rule_based"
      SummarySourceLLM       SummarySource = "llm"
      SummarySourceHybrid    SummarySource = "hybrid"
  )

  type Summary struct {
      Text   string        `json:"text"`
      Source SummarySource `json:"source"`
  }
  ```

- **Narrow `SummaryInput`, `SummaryResult`, & Interface Seam**:
  ```go
  type SummaryInput struct {
      Chunks    []pdfmodel.KnowledgeChunk
      Keywords  []pdfmodel.Keyword
      Entities  []pdfmodel.Entity
      Concepts  []pdfmodel.Concept
      Relations []pdfmodel.Relation
  }

  type SummaryResult struct {
      DocumentSummary *pdfmodel.Summary
      ChunkSummaries  map[string]*pdfmodel.Summary
  }

  type SummaryExtractor interface {
      ExtractSummaries(ctx context.Context, input SummaryInput) (SummaryResult, error)
  }
  ```

- **`SummaryPass` Pipeline Stage**:
  - Requires: `CapabilityKeywords`, `CapabilityEntities`, `CapabilityConcepts`, `CapabilityRelations`
  - Provides: `CapabilitySummary`

- **Dual-Level Attachment Strategy**:
  - **Document Level**: Attached to `DocumentMetadata.Summary`.
  - **Chunk Level**: Attached to `KnowledgeChunk.Summary`.

## Consequences

### Positive
- Extractive MVP is 100% deterministic, zero network cost, fast, and testable.
- Clear separation of concerns between extractor strategies (producing `SummaryResult`) and pass orchestrators (attaching to models).
- Prepares seamless seam for future `LLMSummaryExtractor` implementations.

### Negative
- Initial MVP summaries are extractive sentences rather than fluently synthesized abstractive narratives.
