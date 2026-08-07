package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	retrievalseam "arca/internal/retrieval/seam"
)

// debugWriter is the stderr sink for the per-call debug line; swapped in
// tests to capture structured output.
var debugWriter io.Writer = log.Default().Writer()

// HTTPReranker is the production Reranker seam adapter (M10, ADR-0049): an
// HTTP client depending only on the reranker-service contract
// (POST /rerank: {query, candidates:[{id,text}]} -> {ranked_ids, scores}),
// never on the Python implementation. It carries the minimal runtime
// observability: atomic counters plus one structured debug line per call.
type HTTPReranker struct {
	url     string
	client  *http.Client
	debug   bool

	requests atomic.Int64
	failures atomic.Int64
	latency  atomic.Int64
	lastDegraded atomic.Bool
}

// NewHTTPReranker constructs the adapter. timeout covers only the HTTP call;
// every failure mode (timeout, connection, 5xx, invalid response) surfaces
// as an error so the wrapper's graceful degradation applies uniformly.
func NewHTTPReranker(baseURL string, timeout time.Duration) *HTTPReranker {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HTTPReranker{
		url:   strings.TrimRight(baseURL, "/"),
		client: &http.Client{Timeout: timeout},
		debug: true,
	}
}

// Rerank implements the Reranker seam: sends the candidates to the service,
// maps ranked_ids order to ScoredCandidate order (scores aligned), and
// reports the per-call observability line. Any failure returns an error —
// the wrapper degrades to the inner ordering.
func (r *HTTPReranker) Rerank(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]ScoredCandidate, error) {
	start := time.Now()
	r.requests.Add(1)
	r.lastDegraded.Store(false)

	defer func() {
		latency := time.Since(start).Milliseconds()
		r.latency.Add(latency)
		if r.debug {
			fmt.Fprintf(debugWriter, "reranker: candidate_count=%d reranked_count=%d latency_ms=%d degraded=%v\n",
				len(candidates), len(candidates), latency, r.lastDegraded.Load())
		}
	}()

	ordered, err := r.call(ctx, query, candidates)
	if err != nil {
		r.failures.Add(1)
		r.lastDegraded.Store(true)
		if r.debug {
			fmt.Fprintf(debugWriter, "reranker: error=%v\n", err)
		}
		return nil, err
	}
	return ordered, nil
}

// RequestsTotal reports the total rerank requests (atomic counter).
func (r *HTTPReranker) RequestsTotal() int64 { return r.requests.Load() }

// FailuresTotal reports the total failed rerank calls (atomic counter).
func (r *HTTPReranker) FailuresTotal() int64 { return r.failures.Load() }

// LatencyMsTotal reports the summed rerank latency in milliseconds (atomic).
func (r *HTTPReranker) LatencyMsTotal() int64 { return r.latency.Load() }

// LastDegraded reports whether the last call degraded to the inner ordering.
func (r *HTTPReranker) LastDegraded() bool { return r.lastDegraded.Load() }

type httpRerankRequest struct {
	Query      string           `json:"query"`
	Candidates []httpCandidate `json:"candidates"`
}

type httpCandidate struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type httpRerankResponse struct {
	RankedIDs []string  `json:"ranked_ids"`
	Scores    []float32 `json:"scores"`
}

// call performs one HTTP round trip and validates the contract response.
func (r *HTTPReranker) call(ctx context.Context, query string, candidates []retrievalseam.SearchResult) ([]ScoredCandidate, error) {
	reqBody := httpRerankRequest{Query: query, Candidates: make([]httpCandidate, len(candidates))}
	for i, c := range candidates {
		reqBody.Candidates[i] = httpCandidate{ID: c.ChunkID, Text: c.ContentMarkdown}
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("reranker request marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+"/rerank", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("reranker request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reranker call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reranker response read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reranker status %d", resp.StatusCode)
	}

	var parsed httpRerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("reranker invalid response: %w", err)
	}
	if len(parsed.RankedIDs) == 0 && len(parsed.Scores) == 0 {
		return nil, fmt.Errorf("reranker invalid response: empty ranked_ids/scores")
	}
	if len(parsed.RankedIDs) != len(parsed.Scores) {
		return nil, fmt.Errorf("reranker invalid response: %d ranked_ids vs %d scores", len(parsed.RankedIDs), len(parsed.Scores))
	}

	ordered := make([]ScoredCandidate, len(parsed.RankedIDs))
	for i, id := range parsed.RankedIDs {
		ordered[i] = ScoredCandidate{ChunkID: id, Score: parsed.Scores[i]}
	}
	return ordered, nil
}
