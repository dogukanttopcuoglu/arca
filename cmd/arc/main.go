package main

import (
	"context"
	"fmt"
	"os"

	arccli "arca/cmd/arc/cli"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("ARC Document Intelligence OS CLI")
		fmt.Println("Usage: arc [inspect|ask|research] <args>")
		os.Exit(1)
	}

	app := arccli.NewApp()
	ctx := context.Background()
	cmd := os.Args[1]

	switch cmd {
	case "inspect":
		filePath := "document.pdf"
		if len(os.Args) > 2 {
			filePath = os.Args[2]
		}
		out, err := app.RunInspect(ctx, filePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)

	case "ask":
		query := "What is creativity?"
		if len(os.Args) > 2 {
			query = os.Args[2]
		}
		out, err := app.RunAsk(ctx, query)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)

	case "research":
		query := "Synthesize principles"
		if len(os.Args) > 2 {
			query = os.Args[2]
		}
		out, err := app.RunResearch(ctx, query)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)

	default:
		fmt.Printf("Unknown command %q\n", cmd)
		os.Exit(1)
	}
}
