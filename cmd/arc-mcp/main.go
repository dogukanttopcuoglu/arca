package main

import (
	"fmt"
	"os"

	"arca/cmd/arc-mcp/server"
)

func main() {
	srv := server.NewServer()
	tools := srv.ListTools()

	fmt.Printf("ARC Native MCP Server starting (Registered Tools: %d)\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s: %s\n", t.Name, t.Description)
	}

	// MCP Stdin/Stdout JSON-RPC transport listener
	os.Exit(0)
}
