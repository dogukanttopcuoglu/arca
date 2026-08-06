package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"

	retrievalseam "arca/internal/retrieval/seam"
)

// TestExecRerankerHelperProcess is the model-process stand-in: it consumes
// NDJSON requests on stdin and answers with reversed orderings, reporting
// model load and RSS once. It is invoked by the real tests as a subprocess
// (GO_WANT_HELPER_PROCESS=1); running it as a normal test is a no-op.
func TestExecRerankerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	sc := bufio.NewScanner(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	first := true
	for sc.Scan() {
		var req execRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			w.WriteString(`{"ordering":[]}` + "\n")
			w.Flush()
			continue
		}
		resp := execResponse{}
		for i := len(req.Candidates) - 1; i >= 0; i-- {
			resp.Ordering = append(resp.Ordering, execScored{ChunkID: req.Candidates[i].ChunkID, Score: float32(i)})
		}
		if first {
			resp.ModelLoadMs = 42
			resp.RSSBytes = 1024
			first = false
		}
		raw, _ := json.Marshal(resp)
		w.Write(raw)
		w.WriteByte('\n')
		w.Flush()
	}
	os.Exit(0)
}

func TestExecRerankerRoundTrip(t *testing.T) {
	r := NewExecReranker(os.Args[0], "-test.run=^TestExecRerankerHelperProcess$")
	r.SetEnv("GO_WANT_HELPER_PROCESS=1")
	defer r.Close()

	ordered, err := r.Rerank(context.Background(), "q", []retrievalseam.SearchResult{
		{ChunkID: "a", ContentMarkdown: "first"},
		{ChunkID: "b", ContentMarkdown: "second"},
		{ChunkID: "c", ContentMarkdown: "third"},
	})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(ordered) != 3 || ordered[0].ChunkID != "c" || ordered[2].ChunkID != "a" {
		t.Fatalf("ordering = %+v, want reversed [c b a]", ordered)
	}
	if r.LoadTimeMs() != 42 {
		t.Fatalf("load time = %d, want 42 from first response", r.LoadTimeMs())
	}
	if r.RSSBytes() != 1024 {
		t.Fatalf("rss = %d, want 1024 from first response", r.RSSBytes())
	}
}

func TestExecRerankerReusesProcess(t *testing.T) {
	r := NewExecReranker(os.Args[0], "-test.run=^TestExecRerankerHelperProcess$")
	r.SetEnv("GO_WANT_HELPER_PROCESS=1")
	defer r.Close()

	for i := 0; i < 3; i++ {
		ordered, err := r.Rerank(context.Background(), "q", []retrievalseam.SearchResult{{ChunkID: "x"}, {ChunkID: "y"}})
		if err != nil {
			t.Fatalf("Rerank #%d: %v", i, err)
		}
		if len(ordered) != 2 || ordered[0].ChunkID != "y" {
			t.Fatalf("Rerank #%d ordering = %+v, want [y x]", i, ordered)
		}
	}
	if r.LoadTimeMs() != 42 {
		t.Fatalf("load time = %d, want reported once", r.LoadTimeMs())
	}
}
