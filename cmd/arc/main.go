package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	arccli "arca/cmd/arc/cli"
	retrievalseam "arca/internal/retrieval/seam"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("ARC Document Intelligence OS CLI")
		fmt.Println("Usage: arc [inspect|ask|research|eval] <args>")
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

	case "eval":
		fs := flag.NewFlagSet("eval", flag.ExitOnError)
		goldset := fs.String("goldset", "internal/eval/testdata/goldset_v1.json", "path to the gold set JSON")
		mode := fs.String("mode", "dense", "retrieval mode: dense|sparse|hybrid")
		topk := fs.Int("topk", 5, "top-k retrieval depth")
		minScore := fs.Float64("min-score", 0, "minimum retrieval score threshold")
		report := fs.String("report", "", "path to write the JSON report")
		fusionPolicy := fs.String("fusion-policy", "", "hybrid fusion policy: balanced|densebiased")
		sparseWeight := fs.Float64("sparse-weight", 0, "hybrid sparse stream fusion weight (0 = policy default)")
		sparseCap := fs.Int("sparse-cap", 0, "hybrid sparse candidate cap (0 = unlimited)")
		decompose := fs.Bool("decompose", false, "run gold-set queries through rule-based decomposition")
		m5gate := fs.Bool("m5-gate", false, "run the M5 semantic evidence gate over each query's context")
		if err := fs.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		m, err := parseMode(*mode)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		out, err := app.RunEval(ctx, arccli.EvalOptions{
			GoldSetPath:      *goldset,
			Mode:             m,
			TopK:             *topk,
			MinScore:         float32(*minScore),
			ReportPath:       *report,
			FusionPolicyName: *fusionPolicy,
			SparseWeight:     *sparseWeight,
			SparseCap:        *sparseCap,
			Decompose:        *decompose,
			M5Gate:           *m5gate,
		})
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

// parseMode maps a CLI mode string to the retrieval mode enum.
func parseMode(s string) (retrievalseam.RetrievalMode, error) {
	switch strings.ToLower(s) {
	case "dense":
		return retrievalseam.RetrievalDense, nil
	case "sparse":
		return retrievalseam.RetrievalSparse, nil
	case "hybrid":
		return retrievalseam.RetrievalHybrid, nil
	default:
		return 0, fmt.Errorf("unknown retrieval mode %q", s)
	}
}
