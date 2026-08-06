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
		if len(os.Args) > 2 && os.Args[2] == "probe" {
			runProbe(ctx, app, os.Args[3:])
			return
		}
		if len(os.Args) > 2 && os.Args[2] == "embed-probe" {
			runEmbedProbe(ctx, app, os.Args[3:])
			return
		}
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
		comparisonTopK := fs.Int("comparison-topk", 0, "M6 evidence budget: effective TopK for comparison-intent queries (0 = use --topk)")
		graphWeight := fs.Float64("graph-weight", 0, "M7 graph fusion weight: >0 fuses dense + graph streams (0 = default retriever)")
		graphOnly := fs.Bool("graph-only", false, "M7: measure the graph stream alone")
		gateRuns := fs.Int("gate-runs", 1, "repeat each gate evaluation and record the median decision (stabilizes gate metrics against LLM variance)")
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
			ComparisonTopK:   *comparisonTopK,
			GraphWeight:      *graphWeight,
			GraphOnly:        *graphOnly,
			GateRuns:         *gateRuns,
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

// runProbe dispatches the M8 probe subcommands: `arc eval probe collect`
// records the candidate artifact; `arc eval probe run` simulates rerankers
// and evaluates the kill gate (ADR-0045).
func runProbe(ctx context.Context, app *arccli.App, args []string) {	if len(args) < 1 {
		fmt.Println("Usage: arc eval probe [collect|run] <args>")
		os.Exit(1)
	}
	switch args[0] {
	case "collect":
		fs := flag.NewFlagSet("probe collect", flag.ExitOnError)
		goldset := fs.String("goldset", "", "path to the gold set JSON (required)")
		artifact := fs.String("artifact-out", "", "path to write the candidate artifact (required)")
		candidateTopN := fs.Int("candidate-top-n", 100, "candidate top N recorded by the baseline")
		if err := fs.Parse(args[1:]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		out, err := app.RunProbeCollect(ctx, arccli.ProbeCollectOptions{
			GoldSetPath:   *goldset,
			ArtifactPath:  *artifact,
			CandidateTopN: *candidateTopN,
		})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)

	case "run":
		fs := flag.NewFlagSet("probe run", flag.ExitOnError)
		artifact := fs.String("artifact", "", "path to the candidate artifact (required)")
		goldset := fs.String("goldset", "", "path to the gold set JSON (required)")
		bgeCommand := fs.String("bge-command", "", "command running the BGE cross-encoder reranker script")
		candidateNs := fs.String("n", "20,50,100", "comma-separated candidate budgets N to sweep")
		maxP95 := fs.Int64("budget-p95-ms", 0, "frozen p95 rerank latency budget (ms)")
		maxRSS := fs.Int64("budget-rss-bytes", 0, "frozen model memory budget (bytes)")
		report := fs.String("report", "", "path to write the JSON manifest")
		m5gate := fs.Bool("m5-gate", true, "evaluate the M5 semantic evidence gate per combination (ADR-0045: answer quality is measured on every combination; disable only for ranking-only runs)")
		structure := fs.Bool("structure", false, "add the deterministic structure-bonus reranker (research E2, model-free heading overlap)")
		structureIntents := fs.String("structure-intents", "", "gate the structure reranker to these intents (comma-separated; empty = all)")
		if err := fs.Parse(args[1:]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		var ns []int
		for _, part := range strings.Split(*candidateNs, ",") {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &n); err != nil || n <= 0 {
				fmt.Printf("Error: invalid candidate N %q\n", part)
				os.Exit(1)
			}
			ns = append(ns, n)
		}
		out, err := app.RunProbe(ctx, arccli.ProbeRunOptions{
			ArtifactPath:    *artifact,
			GoldSetPath:     *goldset,
			BGECommand:      *bgeCommand,
			CandidateNs:     ns,
			MaxP95Ms:        *maxP95,
			MaxRSSBytes:     *maxRSS,
			ReportPath:      *report,
			M5Gate:          *m5gate,
			Structure:       *structure,
			StructureIntents: *structureIntents,
		})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(out)

	default:
		fmt.Printf("Unknown probe subcommand %q\n", args[0])
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

// runEmbedProbe dispatches the ADR-0047 embedding representation probe:
// re-embeds the live corpus per representation into probe collections and
// measures Gold Set v3 + v4 (heading slice) against each.
func runEmbedProbe(ctx context.Context, app *arccli.App, args []string) {
	fs := flag.NewFlagSet("eval embed-probe", flag.ExitOnError)
	goldsetV3 := fs.String("goldset-v3", "internal/eval/testdata/goldset_v3.json", "path to Gold Set v3 (regression canary)")
	goldsetV4 := fs.String("goldset-v4", "internal/eval/testdata/goldset_v4.json", "path to Gold Set v4 (heading slice)")
	inspectionDir := fs.String("inspection-dir", "", "directory of inspection results for document titles (optional)")
	reps := fs.String("reps", "a,b,c,d", "representations to probe: a,b,c,d")
	report := fs.String("report", "", "path to write the JSON probe report")
	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	out, err := app.RunEmbedProbe(ctx, arccli.EmbedProbeOptions{
		GoldSetV3Path:   *goldsetV3,
		GoldSetV4Path:   *goldsetV4,
		InspectionDir:   *inspectionDir,
		ReportPath:      *report,
		Representations: *reps,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(out)
}
