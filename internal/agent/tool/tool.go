package tool

import (
	"context"
)

// ToolInput models generic inputs passed to an Agent Tool.
type ToolInput struct {
	Query  string         `json:"query"`
	Params map[string]any `json:"params,omitempty"`
}

// ToolResult models generic outputs returned from an Agent Tool.
type ToolResult struct {
	Summary string `json:"summary"`
	Data    any    `json:"data,omitempty"`
}

// Tool defines the deep module interface seam for system actions callable by AgentEngine.
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input ToolInput) (ToolResult, error)
}
