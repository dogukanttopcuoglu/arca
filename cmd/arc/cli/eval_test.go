package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"arca/internal/eval"
	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/store"
	retrievalseam "arca/internal/retrieval/seam"
)

const (
	hashA = "1111111111111111111111111111111111111111111111111111111111111111"
	hashB = "2222222222222222222222222222222222222222222222222222222222222222"
)

// writeTempGoldSet writes a gold set matching the seeded chunks' fingerprint.
func writeTempGoldSet(t *testing.T, fingerprint string, queries string) string {
	t.Helper()
	content := fmt.Sprintf(`{
  "schema_version": "1",
  "corpus": {
    "document_id": "doc-1",
    "corpus_fingerprint": %q,
    "chunk_count": 2
  },
  "queries": %s
}`, fingerprint, queries)
	path := filepath.Join(t.TempDir(), "goldset.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write gold set: %v", err)
	}
	return path
}

func evalFixture(t *testing.T) (*App, string) {
	t.Helper()
	ctx := context.Background()

	cfg := DefaultConfig()
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("failed to construct runtime: %v", err)
	}

	vecStore := runtime.vectorStore.(*store.InMemoryVectorStore)
	for _, spec := range []struct{ id, hash, text string }{
		{"chk-1", hashA, "Creativity is a fundamental human quality."},
		{"chk-2", hashB, "Discipline and daily practice turn creative impulses into finished works."},
	} {
		vec, err := runtime.embeddingProvider.EmbedQuery(ctx, spec.text)
		if err != nil || len(vec) == 0 {
			t.Fatalf("failed to embed seed chunk: %v", err)
		}
		if err := vecStore.UpsertPoints(ctx, []store.VectorPoint{{
			ID:              "pt-" + spec.id,
			Vector:          vec,
			ContentMarkdown: spec.text,
			Metadata: indexingmodel.VectorMetadata{
				DocumentID:  "doc-1",
				ChunkID:     spec.id,
				SectionPath: "Introduction",
				ContentHash: spec.hash,
			},
		}}); err != nil {
			t.Fatalf("failed to seed vector store: %v", err)
		}
		if err := runtime.contentStore.PutContent(ctx, []store.ChunkContent{
			{ChunkID: spec.id, ContentMarkdown: spec.text},
		}); err != nil {
			t.Fatalf("failed to seed content store: %v", err)
		}
	}

	fingerprint := eval.ComputeFingerprint([]string{hashA, hashB})
	queries := `[
    {"id": "q1", "intent": "concept", "query": "What is creativity?", "expected_chunk_ids": ["chk-1", "chk-2"], "expected_sections": ["Introduction"], "expected_no_evidence": false}
  ]`
	goldsetPath := writeTempGoldSet(t, fingerprint, queries)

	return NewAppWithRuntime(runtime), goldsetPath
}

func TestAppRunEval(t *testing.T) {
	ctx := context.Background()

	t.Run("runs the benchmark, renders a table, and writes the report", func(t *testing.T) {
		app, goldsetPath := evalFixture(t)
		reportPath := filepath.Join(t.TempDir(), "report.json")

		out, err := app.RunEval(ctx, EvalOptions{
			GoldSetPath: goldsetPath,
			Mode:        retrievalseam.RetrievalDense,
			TopK:        5,
			ReportPath:  reportPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "AGGREGATE") || !strings.Contains(out, "q1") {
			t.Errorf("expected aggregate table with query rows, got:\n%s", out)
		}
		if !strings.Contains(out, "recall@5") || !strings.Contains(out, "no_evidence_precision") {
			t.Errorf("expected metric columns in table, got:\n%s", out)
		}

		raw, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatalf("report file not written: %v", err)
		}
		var report eval.Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatalf("report is not valid JSON: %v", err)
		}
		if report.Metrics.RecallAtK != 1.0 {
			t.Errorf("expected recall@5 1.0 for fully relevant retrieval, got %v", report.Metrics.RecallAtK)
		}
		if report.Corpus.ChunkCount != 2 || report.Retrieval.Mode != "Dense" {
			t.Errorf("report manifest incomplete: %+v", report.Corpus)
		}
	})

	t.Run("fails on fingerprint mismatch with no report", func(t *testing.T) {
		app, _ := evalFixture(t)
		badFingerprint := strings.Repeat("f", 64)
		queries := `[
    {"id": "q1", "intent": "concept", "query": "What is creativity?", "expected_chunk_ids": ["chk-1"], "expected_sections": [], "expected_no_evidence": false}
  ]`
		goldsetPath := writeTempGoldSet(t, badFingerprint, queries)

		_, err := app.RunEval(ctx, EvalOptions{GoldSetPath: goldsetPath, Mode: retrievalseam.RetrievalDense, TopK: 5})
		if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
			t.Fatalf("expected fingerprint mismatch error, got %v", err)
		}
	})

	t.Run("runs sparse and hybrid modes against the same gold set", func(t *testing.T) {
		app, goldsetPath := evalFixture(t)

		for _, mode := range []retrievalseam.RetrievalMode{retrievalseam.RetrievalSparse, retrievalseam.RetrievalHybrid} {
			out, err := app.RunEval(ctx, EvalOptions{
				GoldSetPath: goldsetPath,
				Mode:        mode,
				TopK:        5,
			})
			if err != nil {
				t.Fatalf("mode %s failed: %v", mode, err)
			}
			if !strings.Contains(out, "AGGREGATE") {
				t.Errorf("expected aggregate output for mode %s, got:\n%s", mode, out)
			}
		}
	})
}
