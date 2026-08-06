package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"arca/internal/eval"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
	pdfmodel "arca/internal/pdfinspector/model"
	"arca/internal/retrieval/dense"
	retrievalseam "arca/internal/retrieval/seam"
)

// EmbedProbeOptions configures the ADR-0047 embedding representation probe:
// representations A/B/C/D are re-embedded into separate probe collections and
// measured against Gold Set v3 (regression canary) and v4 (heading slice).
type EmbedProbeOptions struct {
	GoldSetV3Path   string
	GoldSetV4Path   string
	InspectionDir   string // optional: docID -> title map for BookPath representation
	ReportPath      string
	Representations string // "a,b,c,d" (default all)
}

// ProbeRepResult is one representation's metrics on both gold sets.
type ProbeRepResult struct {
	Representation string    `json:"representation"`
	Collection     string    `json:"collection"`
	V3             eval.Metrics `json:"goldset_v3"`
	V4             eval.Metrics `json:"goldset_v4"` // heading slice
}

// EmbedProbeReport aggregates the representation comparison (ADR-0047 gate:
// v4 heading MPI vs A, v3 MAR vs A).
type EmbedProbeReport struct {
	GitCommit string           `json:"git_commit"`
	Results   []ProbeRepResult `json:"results"`
	// Gate is filled after EvaluateEmbedProbe; see gate rules below.
	Gate EmbedProbeGate `json:"gate"`
}

// EmbedProbeGate is the frozen ADR-0047 acceptance evaluation.
type EmbedProbeGate struct {
	MPI            float64 `json:"mpi_ndcg5_pp"`           // +1 pp on v4 heading slice
	MAR            float64 `json:"mar_ndcg5_rel"`          // 5% relative regression on v3
	Accepted       bool    `json:"accepted"`
	Selected       string  `json:"selected"`               // winning representation
	Reason         string  `json:"reason"`
}

const (
	probeMPI = 1.0   // nDCG@5 delta in percentage points on v4 heading slice
	probeMAR = 0.05  // 5% relative regression on v3 aggregate
)

// RunEmbedProbe re-embeds the live corpus per representation into a probe
// collection and runs both gold sets against it.
func (a *App) RunEmbedProbe(ctx context.Context, opts EmbedProbeOptions) (string, error) {
	reps := parseRepresentations(opts.Representations)
	if len(reps) == 0 {
		return "", fmt.Errorf("no representations selected (--reps a,b,c,d)")
	}

	gsV3, err := loadGoldSetFile(opts.GoldSetV3Path)
	if err != nil {
		return "", err
	}
	gsV4, err := loadGoldSetFile(opts.GoldSetV4Path)
	if err != nil {
		return "", err
	}

	// Canonical corpus: read every indexed chunk from the live collection.
	points, err := a.runtime.vectorStore.ListPoints(ctx, indexingmodel.MetadataFilter{})
	if err != nil {
		return "", fmt.Errorf("failed to read live corpus: %w", err)
	}
	byDoc := map[string][]pdfmodel.KnowledgeChunk{}
	for _, p := range points {
		chk := pdfmodel.KnowledgeChunk{
			ChunkID:         p.Metadata.ChunkID,
			DocumentID:      p.Metadata.DocumentID,
			ChunkOrder:      p.Metadata.ChunkOrder,
			SectionPath:     p.Metadata.SectionPath,
			PageNumbers:     p.Metadata.PageNumbers,
			ContentType:     p.Metadata.ContentType,
			ContentHash:     p.Metadata.ContentHash,
			ContentMarkdown: p.ContentMarkdown,
		}
		byDoc[chk.DocumentID] = append(byDoc[chk.DocumentID], chk)
	}
	var docIDs []string
	for id := range byDoc {
		docIDs = append(docIDs, id)
	}
	sort.Strings(docIDs)

	titles := loadProbeTitles(opts.InspectionDir)

	host := strings.TrimPrefix(strings.TrimPrefix(a.runtime.cfg.VectorStoreURL, "http://"), "https://")
	topK := 5
	minScore := a.runtime.cfg.RetrievalMinScore

	report := &EmbedProbeReport{GitCommit: gitHead()}

	for _, rep := range reps {
		letter := repLetter(rep)
		collection := "arca_probe_" + letter
		probeStore, err := store.NewQdrantVectorStore(host, collection)
		if err != nil {
			return "", fmt.Errorf("probe collection %s: %w", collection, err)
		}
		probeContent := store.NewInMemoryContentStore()

		w := worker.NewIndexingWorker(
			a.runtime.embeddingProvider,
			probeStore,
			probeContent,
			worker.WithEmbeddingInputRepresentation(rep),
		)
		for _, docID := range docIDs {
			job, err := w.ExecuteSync(ctx, docID, titles[docID], byDoc[docID])
			if err != nil {
				return "", fmt.Errorf("representation %s indexing %s failed: %w", letter, docID, err)
			}
			if job.IndexedChunks == 0 && job.SkippedChunks == 0 {
				return "", fmt.Errorf("representation %s processed nothing for %s (diff anomaly)", letter, docID)
			}
		}

		denseRet := dense.NewDenseRetriever(a.runtime.embeddingProvider, probeStore, probeContent)
		runner := eval.New(denseRet, listPointsSource{store: probeStore}, eval.Options{
			Mode:              retrievalseam.RetrievalDense,
			TopK:              topK,
			MinScore:          minScore,
			EmbeddingProvider: a.runtime.embeddingProvider.Provider(),
			EmbeddingModel:    a.runtime.embeddingProvider.Model(),
			Collection:        collection,
			GitCommit:         gitHead(),
			ComparisonTopK:    a.runtime.cfg.ComparisonTopK,
		})

		r3, err := runner.Run(ctx, gsV3)
		if err != nil {
			return "", fmt.Errorf("representation %s goldset v3: %w", letter, err)
		}
		r4, err := runner.Run(ctx, gsV4)
		if err != nil {
			return "", fmt.Errorf("representation %s goldset v4: %w", letter, err)
		}

		report.Results = append(report.Results, ProbeRepResult{
			Representation: letter,
			Collection:     collection,
			V3:             r3.Metrics,
			V4:             r4.Metrics,
		})
	}

	report.Gate = evaluateEmbedProbe(report)

	if opts.ReportPath != "" {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return "", fmt.Errorf("serialize probe report: %w", err)
		}
		if err := os.WriteFile(opts.ReportPath, raw, 0644); err != nil {
			return "", fmt.Errorf("write probe report: %w", err)
		}
	}

	return renderEmbedProbe(report), nil
}

// evaluateEmbedProbe applies the ADR-0047 gate: representation D (or the best
// accepted) must gain >= +1 pp nDCG@5 on the v4 heading slice over A and must
// not regress v3 beyond the 5% relative tolerance. A is the baseline.
func evaluateEmbedProbe(report *EmbedProbeReport) EmbedProbeGate {
	gate := EmbedProbeGate{MPI: probeMPI, MAR: probeMAR}
	var base *ProbeRepResult
	for i := range report.Results {
		if report.Results[i].Representation == "a" {
			base = &report.Results[i]
		}
	}
	if base == nil || len(report.Results) < 2 {
		gate.Reason = "no baseline (A) measured"
		return gate
	}

	best := ""
	bestScore := -1.0
	for i := range report.Results {
		r := &report.Results[i]
		if r.Representation == "a" {
			continue
		}
		v4DeltaPp := (r.V4.NDCGAtK - base.V4.NDCGAtK) * 100
		v3Reg := 0.0
		if base.V3.NDCGAtK > 0 {
			v3Reg = (base.V3.NDCGAtK - r.V3.NDCGAtK) / base.V3.NDCGAtK
		}
		if v4DeltaPp < probeMPI {
			continue
		}
		if v3Reg > probeMAR {
			continue
		}
		if r.V4.NDCGAtK > bestScore {
			bestScore = r.V4.NDCGAtK
			best = r.Representation
		}
	}

	if best == "" {
		gate.Reason = fmt.Sprintf(
			"no representation passed the gate (MPI %.1f pp on v4 heading; MAR %.0f%% relative on v3)", probeMPI, probeMAR*100)
		return gate
	}
	gate.Accepted = true
	gate.Selected = best
	gate.Reason = fmt.Sprintf(
		"representation %s passes the ADR-0047 gate (v4 heading MPI >= %.1f pp, v3 MAR <= %.0f%%)", best, probeMPI, probeMAR*100)
	return gate
}

// parseRepresentations maps CLI letters to worker representations.
func parseRepresentations(s string) []worker.EmbeddingInputRepresentation {
	if strings.TrimSpace(s) == "" {
		s = "a,b,c,d"
	}
	var out []worker.EmbeddingInputRepresentation
	for _, part := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "a":
			out = append(out, worker.RepresentationContent)
		case "b":
			out = append(out, worker.RepresentationSectionTitle)
		case "c":
			out = append(out, worker.RepresentationSectionPath)
		case "d":
			out = append(out, worker.RepresentationBookPath)
		}
	}
	return out
}

// repLetter returns the canonical letter for a representation.
func repLetter(rep worker.EmbeddingInputRepresentation) string {
	switch rep {
	case worker.RepresentationSectionTitle:
		return "b"
	case worker.RepresentationSectionPath:
		return "c"
	case worker.RepresentationBookPath:
		return "d"
	default:
		return "a"
	}
}

// loadProbeTitles reads document titles from inspection results (optional).
// Missing documents fall back to the document ID inside the embedding builder.
func loadProbeTitles(dir string) map[string]string {
	titles := map[string]string{}
	if dir == "" {
		return titles
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return titles
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var result pdfmodel.PDFInspectionResult
		if err := json.Unmarshal(raw, &result); err != nil {
			continue
		}
		if result.Document.DocumentID != "" && result.Document.Title != "" {
			titles[result.Document.DocumentID] = result.Document.Title
		}
	}
	return titles
}

func renderEmbedProbe(report *EmbedProbeReport) string {
	var sb strings.Builder
	sb.WriteString("=== ARC ADR-0047 EMBEDDING REPRESENTATION PROBE ===\n")
	fmt.Fprintf(&sb, "%-5s %-18s %-10s %-10s | %-10s %-10s %-10s\n",
		"rep", "collection", "v3 recall", "v3 nDCG", "v4 recall", "v4 MRR", "v4 nDCG")
	for _, r := range report.Results {
		fmt.Fprintf(&sb, "%-5s %-18s %-10.3f %-10.3f | %-10.3f %-10.3f %-10.3f\n",
			r.Representation, r.Collection,
			r.V3.RecallAtK, r.V3.NDCGAtK,
			r.V4.RecallAtK, r.V4.MRR, r.V4.NDCGAtK)
	}
	sb.WriteString("\n")
	if report.Gate.Accepted {
		fmt.Fprintf(&sb, "GATE: ACCEPT — %s\n", report.Gate.Reason)
	} else {
		fmt.Fprintf(&sb, "GATE: REJECT — %s\n", report.Gate.Reason)
	}
	return sb.String()
}

