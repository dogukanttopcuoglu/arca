# 0018: External Interface & Deployment Boundary Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

ARC must be exposed to external clients, AI assistants (Claude Desktop, Cursor, Poke), Web/Mobile applications, and developer CLI workflows without leaking delivery framework details into core domain logic.

## Decision Drivers

- **Hexagonal / Clean Architecture**: Delivery adapters in `cmd/` must wrap `internal/` core modules without `internal/` depending on delivery frameworks.
- **AI Ecosystem Integration**: Native Model Context Protocol (MCP) server support is a first-class entrypoint.

## Decided Options

### Option A: Core Engine + 3 Delivery Adapters (ACCEPTED)
- **Core Engine (`internal/`)**: Independent Knowledge OS core containing all domain logic.
- **Delivery Adapters (`cmd/`)**:
  1. `cmd/arc-mcp`: Native MCP Server exposing tools (`inspect_pdf`, `search_knowledge_space`, `traverse_graph`, `ask_verified_question`, `run_agent_research`) for Claude Desktop, Cursor, and Poke.
  2. `cmd/arc-server`: Fiber HTTP REST API & SSE Streaming server for Web/Mobile/SaaS clients.
  3. `cmd/arc`: Standalone CLI tool for local offline dev, scripting, and CI/CD pipelines.

## Consequences

### Positive
- Identical domain logic is shared across CLI, Web REST/SSE, and MCP AI ecosystem tools.
- Zero delivery framework leakage into core domain logic.
