package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	retrievalseam "arca/internal/retrieval/seam"
	"arca/internal/retrieval/rerank"
)

// ExecReranker is a probe-side Reranker adapter that delegates scoring to an
// external command over NDJSON on stdin/stdout (e.g. a Python model script —
// benchmark tooling only, ADR-0042; production code never runs it). The
// process is spawned lazily on the first Rerank and reused for subsequent
// calls, so model load time (cold) and warm latencies are measured truthfully.
type ExecReranker struct {
	command []string
	env     []string

	mu       sync.Mutex
	proc     *exec.Cmd
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	loadMs   int64
	rssBytes int64
}

// NewExecReranker constructs the adapter. The command receives one NDJSON
// request line per query on stdin and must answer with one NDJSON response
// line on stdout:
//
//	request:  {"query": "...", "candidates": [{"chunk_id": "...", "content": "..."}]}
//	response: {"ordering": [{"chunk_id": "...", "score": 0.9}], "model_load_ms": 123, "rss_bytes": 456}
//
// model_load_ms and rss_bytes are reported once (first response); their
// absence is fine.
func NewExecReranker(command ...string) *ExecReranker {
	return &ExecReranker{command: command}
}

// SetEnv appends environment entries for the spawned process.
func (e *ExecReranker) SetEnv(entries ...string) {
	e.env = append(e.env, entries...)
}

// Rerank serializes the candidates, exchanges one NDJSON line with the model
// process, and returns the reranked ordering. The process is spawned on the
// first call (cold start included in that call's latency).
func (e *ExecReranker) Rerank(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]rerank.ScoredCandidate, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	req := execRequest{Query: query}
	for _, c := range candidates {
		req.Candidates = append(req.Candidates, execCandidate{ChunkID: c.ChunkID, Content: c.ContentMarkdown})
	}

	resp, err := e.roundTrip(ctx, req)
	if err != nil {
		return nil, err
	}

	out := make([]rerank.ScoredCandidate, 0, len(resp.Ordering))
	for _, o := range resp.Ordering {
		out = append(out, rerank.ScoredCandidate{ChunkID: o.ChunkID, Score: o.Score})
	}
	return out, nil
}

// LoadTimeMs returns the model load time reported by the first response.
func (e *ExecReranker) LoadTimeMs() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.loadMs
}

// RSSBytes returns the last reported process memory footprint.
func (e *ExecReranker) RSSBytes() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rssBytes
}

// Close terminates the model process, if any.
func (e *ExecReranker) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.proc == nil {
		return nil
	}
	e.stdin.Close()
	err := e.proc.Wait()
	e.proc = nil
	return err
}

func (e *ExecReranker) roundTrip(ctx context.Context, req execRequest) (execResponse, error) {
	var resp execResponse
	if e.proc == nil {
		if err := e.spawn(); err != nil {
			return resp, err
		}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return resp, fmt.Errorf("marshal request: %w", err)
	}
	if _, err := e.stdin.Write(append(raw, '\n')); err != nil {
		return resp, fmt.Errorf("write request: %w", err)
	}
	if !e.scanner.Scan() {
		if err := e.scanner.Err(); err != nil {
			return resp, fmt.Errorf("read response: %w", err)
		}
		return resp, fmt.Errorf("model process closed the stream")
	}
	if ctx.Err() != nil {
		return resp, ctx.Err()
	}
	if err := json.Unmarshal(e.scanner.Bytes(), &resp); err != nil {
		return resp, fmt.Errorf("invalid response line: %w", err)
	}
	if e.loadMs == 0 && resp.ModelLoadMs > 0 {
		e.loadMs = resp.ModelLoadMs
	}
	if resp.RSSBytes > 0 {
		e.rssBytes = resp.RSSBytes
	}
	return resp, nil
}

func (e *ExecReranker) spawn() error {
	e.proc = exec.Command(e.command[0], e.command[1:]...)
	e.proc.Env = append(os.Environ(), e.env...)
	stdin, err := e.proc.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := e.proc.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	e.proc.Stderr = os.Stderr
	if err := e.proc.Start(); err != nil {
		return fmt.Errorf("spawn model process: %w", err)
	}
	e.stdin = stdin
	e.scanner = bufio.NewScanner(stdout)
	e.scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return nil
}

type execRequest struct {
	Query      string           `json:"query"`
	Candidates []execCandidate  `json:"candidates"`
}

type execCandidate struct {
	ChunkID string `json:"chunk_id"`
	Content string `json:"content"`
}

type execResponse struct {
	Ordering    []execScored `json:"ordering"`
	ModelLoadMs int64        `json:"model_load_ms,omitempty"`
	RSSBytes    int64        `json:"rss_bytes,omitempty"`
}

type execScored struct {
	ChunkID string  `json:"chunk_id"`
	Score   float32 `json:"score"`
}
