package main_test

import (
	"context"
	"testing"

	arcmcp "arca/cmd/arc-mcp/server"
)

func TestMCPServerTools(t *testing.T) {
	ctx := context.Background()
	srv := arcmcp.NewServer()

	t.Run("lists available native MCP tools", func(t *testing.T) {
		tools := srv.ListTools()
		if len(tools) < 5 {
			t.Fatalf("expected at least 5 registered MCP tools, got %d", len(tools))
		}

		expectedTools := map[string]bool{
			"inspect_pdf":              false,
			"search_knowledge_space":   false,
			"traverse_knowledge_graph": false,
			"ask_verified_question":    false,
			"run_agent_research":       false,
		}

		for _, tool := range tools {
			if _, exists := expectedTools[tool.Name]; exists {
				expectedTools[tool.Name] = true
			}
		}

		for name, found := range expectedTools {
			if !found {
				t.Errorf("expected registered tool %q not found", name)
			}
		}
	})

	t.Run("executes native MCP tool 'ask_verified_question'", func(t *testing.T) {
		res, err := srv.ExecuteTool(ctx, "ask_verified_question", map[string]any{
			"query": "What is creativity?",
		})
		if err != nil {
			t.Fatalf("unexpected error executing tool: %v", err)
		}

		if res == nil {
			t.Fatal("expected non-nil ToolResult")
		}
		if res.Content == "" {
			t.Error("expected non-empty Content in ToolResult")
		}
	})
}
