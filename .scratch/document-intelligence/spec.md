# Technical Specification: Milestone 2 — Document Intelligence Operating System

- **Author:** Staff Software Architect
- **Status:** Draft / Accepted
- **Created:** 2026-08-02
- **Target Branch:** `main`

---

## 1. Executive Summary

Milestone 2 evolves ARC from a high-precision Knowledge Indexing & Retrieval engine into an end-to-end **Document Intelligence Operating System**. It introduces RAG Pipeline Orchestration, Context Assembly with Token Budgeting, Vendor-Agnostic Prompt & LLM Abstractions, Inline Citation Extraction & Verification, Dual-Mode Execution (SSE Streaming + Async QA Jobs), 3-Tier Multi-Document Knowledge Spaces, GraphRAG Bounded Context, Agentic Research Orchestration, and Native MCP Server integration.

---

## 2. Architectural Principles & ADR Summary

1. **ADR-0009 (RAG Orchestration Pipeline)**: `AnswerEngine` uses Modular Pipeline Composition with independent seams.
2. **ADR-0010 (Context Assembly Boundary)**: `ContextBuilder` in `internal/qa/context` manages token budgeting and immutable reference keys (`[Ref N]`).
3. **ADR-0011 (Prompt Assembly & LLM Provider Boundary)**: System instructions and RAG citation rules live in `internal/qa/prompt`. `LLMProvider` in `internal/llm/provider` is a pure transport adapter.
4. **ADR-0012 (Citation Extraction & Verification)**: System uses inline reference markers `[Ref N]` + post-processing `CitationExtractor` in `internal/qa/citation`.
5. **ADR-0013 (Evidence Verification & Entailment Seam)**: Phase 1 (Structural Verification MVP) + Phase 2 (`EntailmentChecker` NLI seam) in `internal/qa/verification`.
6. **ADR-0014 (RAG Execution Model)**: Dual-mode execution via `StreamingAnswerEngine` (SSE) and `AsyncAnswerEngine` (`QAJob` state machine).
7. **ADR-0015 (Knowledge Boundary & Multi-Document Isolation)**: 3-tier hierarchy (`Workspace` -> `KnowledgeSpace` -> `Document` -> `Chunk`) with `SecurityContext` isolation.
8. **ADR-0016 (Knowledge Graph Bounded Context)**: `internal/graph` bounded context with `GraphStore`, `GraphRetriever` (3-way RRF fusion), and `EntityExtractor`.
9. **ADR-0017 (Agentic Knowledge Layer)**: `internal/agent` ReAct Re-search controller (`Planner` + `Executor`) wrapping system capabilities as `Tool` seams with security policies (`MaxSteps`, `ApprovalSeam`).
10. **ADR-0018 (External Interface & Deployment)**: Core engine in `internal/` exposed via 3 delivery adapters: `cmd/arc` (CLI), `cmd/arc-server` (Fiber HTTP/REST/SSE), and `cmd/arc-mcp` (Native MCP Server).

---

## 3. Directory Layout

```
arca/
├── cmd/
│   ├── arc/                # CLI tool
│   ├── arc-server/         # Fiber HTTP/REST/SSE server
│   └── arc-mcp/            # Native Model Context Protocol server
├── internal/
│   ├── pdfinspector/       # PDF inspection core pipeline
│   ├── indexing/           # Differential indexing, vector store & worker
│   ├── retrieval/          # Dense, Sparse, Hybrid retrieval & seam
│   ├── qa/                 # RAG Orchestration Core
│   │   ├── context/        # Token budgeting, ContextBuilder & formatting
│   │   ├── prompt/         # System instructions & PromptBuilder
│   │   ├── citation/       # Inline marker extraction & mapper
│   │   ├── verification/   # Structural verifier & Entailment seam
│   │   └── job/            # Async QAJob state machine & worker
│   ├── llm/                # LLMProvider seam & adapters (OpenAI, Anthropic, Ollama, Mock)
│   ├── graph/              # Knowledge Graph bounded context (Nodes, Edges, GraphStore, GraphRetriever)
│   ├── agent/              # Autonomous ReAct Re-search controller & Tool seams
│   └── security/           # SecurityContext & Tenant isolation
└── docs/
    └── adr/                # System ADRs (0001 - 0018)
```
