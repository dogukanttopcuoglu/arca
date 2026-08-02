# 09 — Native Model Context Protocol (MCP) Server

**What to build:**
The `cmd/arc-mcp` entrypoint exposing ARC core capabilities as native MCP Tools and Resources (`inspect_pdf`, `search_knowledge_space`, `traverse_knowledge_graph`, `ask_verified_question`, `run_agent_research`) for Claude Desktop, Cursor, and Poke integration.

**Blocked by:** 08 — Agent Engine Foundation.

**Status:** ready-for-agent

- [ ] Implement `cmd/arc-mcp/main.go` entrypoint using standard MCP protocol transport.
- [ ] Register `inspect_pdf` MCP Tool wrapping `internal/pdfinspector`.
- [ ] Register `search_knowledge_space` MCP Tool wrapping `internal/retrieval`.
- [ ] Register `traverse_knowledge_graph` MCP Tool wrapping `internal/graph`.
- [ ] Register `ask_verified_question` MCP Tool wrapping `internal/qa`.
- [ ] Register `run_agent_research` MCP Tool wrapping `internal/agent`.
- [ ] Add integration tests verifying MCP tool execution over stdin/stdout transport.
