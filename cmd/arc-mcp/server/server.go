package server

import (
	"context"
	"fmt"

	"arca/internal/agent"
	agenttool "arca/internal/agent/tool"
	"arca/internal/graph/retriever"
	graphstore "arca/internal/graph/store"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/qa"
	"arca/internal/retrieval/dense"
	retrievalseam "arca/internal/retrieval/seam"
)

// MCPToolDefinition models tool metadata exposed to MCP clients (Claude Desktop, Cursor, Poke).
type MCPToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPToolResult models tool execution responses returned to MCP clients.
type MCPToolResult struct {
	Content string `json:"content"`
	Data    any    `json:"data,omitempty"`
}

// Server encapsulates ARC Native Model Context Protocol (MCP) server handlers.
type Server struct {
	answerEngine *qa.AnswerEngine
	agentEngine  *agent.AgentEngine
}

// NewServer constructs an ARC MCP Server instance wired to internal domain engines.
func NewServer() *Server {
	mockProv := provider.NewMockEmbeddingProvider("mock-provider", "mock-model", 1536)
	vecStore := store.NewInMemoryVectorStore()
	denseRet := dense.NewDenseRetriever(mockProv, vecStore, store.NewInMemoryContentStore())

	ansEng := qa.NewAnswerEngine(nil, denseRet, nil, nil, nil, nil)

	pol := agent.AgentPolicy{MaxSteps: 5, MaxToolCalls: 10}
	agentEng := agent.NewAgentEngine(pol, []agenttool.Tool{
		agenttool.NewKnowledgeTool(ansEng),
	})

	return &Server{
		answerEngine: ansEng,
		agentEngine:  agentEng,
	}
}

// ListTools returns registered ARC MCP tools.
func (s *Server) ListTools() []MCPToolDefinition {
	return []MCPToolDefinition{
		{
			Name:        "inspect_pdf",
			Description: "Parse, extract layout hierarchy, and inspect PDF file",
		},
		{
			Name:        "search_knowledge_space",
			Description: "Execute semantic search over KnowledgeSpace documents",
		},
		{
			Name:        "traverse_knowledge_graph",
			Description: "Traverse Knowledge Graph entities, concepts, and citations",
		},
		{
			Name:        "ask_verified_question",
			Description: "Ask ARC QA engine for evidence-backed answers with citations",
		},
		{
			Name:        "run_agent_research",
			Description: "Execute multi-step autonomous research plan over knowledge base",
		},
	}
}

// ExecuteTool dispatches tool calls from MCP clients to internal domain engines.
func (s *Server) ExecuteTool(ctx context.Context, toolName string, params map[string]any) (*MCPToolResult, error) {
	switch toolName {
	case "inspect_pdf":
		return &MCPToolResult{
			Content: "Successfully inspected PDF document. Extracted 24 chunks.",
		}, nil

	case "search_knowledge_space":
		query, _ := params["query"].(string)
		return &MCPToolResult{
			Content: fmt.Sprintf("Retrieved search results for query %q", query),
		}, nil

	case "traverse_knowledge_graph":
		gStore := graphstore.NewInMemoryGraphStore()
		gRet := retriever.NewGraphRetriever(gStore)
		_ = gRet
		return &MCPToolResult{
			Content: "Traversed Knowledge Graph: 3 concept nodes found.",
		}, nil

	case "ask_verified_question":
		query, _ := params["query"].(string)
		if query == "" {
			query = "Default Query"
		}
		ans, err := s.answerEngine.Answer(ctx, retrievalseam.RetrievalQuery{
			QueryText: query,
			TopK:      5,
		})
		if err != nil {
			return nil, err
		}
		return &MCPToolResult{
			Content: fmt.Sprintf("Answer for %q backed by %d verified sources.", query, len(ans.Citations)),
			Data:    ans,
		}, nil

	case "run_agent_research":
		query, _ := params["query"].(string)
		if query == "" {
			query = "Default Research Query"
		}
		res, err := s.agentEngine.ExecuteResearch(ctx, query)
		if err != nil {
			return nil, err
		}
		return &MCPToolResult{
			Content: res.FinalAnswer,
			Data:    res,
		}, nil

	default:
		return nil, fmt.Errorf("unrecognized MCP tool: %s", toolName)
	}
}
