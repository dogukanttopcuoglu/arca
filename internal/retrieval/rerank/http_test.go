package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	retrievalseam "arca/internal/retrieval/seam"
)

// rerankServer is a configurable fake of the POST /rerank contract.
type rerankServer struct {
	status       int
	body         string
	delay        time.Duration
	requests     atomic.Int64
	lastBody     string
}

func (s *rerankServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		var buf strings.Builder
		buf.ReadFrom(r.Body)
		s.lastBody = buf.String()
		if s.delay > 0 {
			time.Sleep(s.delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		if s.body != "" {
			w.Write([]byte(s.body))
		}
	}
}

func candidates(ids ...string) []retrievalseam.SearchResult {
	out := make([]retrievalseam.SearchResult, len(ids))
	for i, id := range ids {
		out[i] = retrievalseam.SearchResult{ChunkID: id, ContentMarkdown: "text " + id}
	}
	return out
}

func TestHTTPReranker_Contract(t *testing.T) {
	t.Run("ranked_ids order maps to ScoredCandidate order with aligned scores", func(t *testing.T) {
		srv := &rerankServer{status: 200, body: `{"ranked_ids": ["b", "a", "c"], "scores": [0.9, 0.5, 0.1]}`}
		ts := httptest.NewServer(srv.handler())
		defer ts.Close()

		r := NewHTTPReranker(ts.URL, 5*time.Second)
		ordered, err := r.Rerank(context.Background(), "q", candidates("a", "b", "c"))
		if err != nil {
			t.Fatalf("Rerank: %v", err)
		}
		if len(ordered) != 3 || ordered[0].ChunkID != "b" || ordered[1].ChunkID != "a" || ordered[2].ChunkID != "c" {
			t.Fatalf("ordering = %+v, want [b a c]", ordered)
		}
		if ordered[0].Score != 0.9 || ordered[2].Score != 0.1 {
			t.Fatalf("scores not aligned: %+v", ordered)
		}
	})

	t.Run("request body carries query and candidates, never top_k", func(t *testing.T) {
		srv := &rerankServer{status: 200, body: `{"ranked_ids": ["a"], "scores": [1.0]}`}
		ts := httptest.NewServer(srv.handler())
		defer ts.Close()

		r := NewHTTPReranker(ts.URL, 5*time.Second)
		if _, err := r.Rerank(context.Background(), "bu kitap ne anlatıyor", candidates("a", "b")); err != nil {
			t.Fatalf("Rerank: %v", err)
		}
		var req struct {
			Query      string         `json:"query"`
			Candidates []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"candidates"`
			TopK *int `json:"top_k"`
		}
		if err := json.Unmarshal([]byte(srv.lastBody), &req); err != nil {
			t.Fatalf("request body not JSON: %v (%s)", err, srv.lastBody)
		}
		if req.Query != "bu kitap ne anlatıyor" || len(req.Candidates) != 2 || req.Candidates[0].ID != "a" || req.Candidates[0].Text != "text a" {
			t.Fatalf("request = %+v", req)
		}
		if req.TopK != nil {
			t.Fatalf("top_k must never be sent, got %d", *req.TopK)
		}
	})
}

func TestHTTPReranker_FailOpenMatrix(t *testing.T) {
	// All four failure classes must produce the identical degrade behavior:
	// a non-nil error with no partial output.
	tt := []struct {
		name   string
		server *rerankServer
	}{
		{"5xx response", &rerankServer{status: 500, body: `{"detail": "boom"}`}},
		{"invalid JSON", &rerankServer{status: 200, body: `not json`}},
		{"missing fields", &rerankServer{status: 200, body: `{"ok": true}`}},
		{"length mismatch", &rerankServer{status: 200, body: `{"ranked_ids": ["a"], "scores": []}`}},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.server.handler())
			defer ts.Close()

			r := NewHTTPReranker(ts.URL, 5*time.Second)
			ordered, err := r.Rerank(context.Background(), "q", candidates("a", "b"))
			if err == nil {
				t.Fatalf("expected error for %s, got %+v", tc.name, ordered)
			}
			if len(ordered) != 0 {
				t.Fatalf("failure must produce no partial output, got %+v", ordered)
			}
		})
	}

	t.Run("connection refused", func(t *testing.T) {
		// Port 1 is never listening on any host.
		r := NewHTTPReranker("http://127.0.0.1:1", 2*time.Second)
		_, err := r.Rerank(context.Background(), "q", candidates("a"))
		if err == nil {
			t.Fatal("expected connection error, got nil")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := &rerankServer{status: 200, body: `{"ranked_ids": ["a"], "scores": [1.0]}`, delay: 200 * time.Millisecond}
		ts := httptest.NewServer(srv.handler())
		defer ts.Close()

		r := NewHTTPReranker(ts.URL, 20*time.Millisecond)
		_, err := r.Rerank(context.Background(), "q", candidates("a"))
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})
}

func TestHTTPReranker_Observability(t *testing.T) {
	srv := &rerankServer{status: 200, body: `{"ranked_ids": ["b", "a"], "scores": [0.9, 0.4]}`}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	r := NewHTTPReranker(ts.URL, 5*time.Second)
	if _, err := r.Rerank(context.Background(), "q", candidates("a", "b")); err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if _, err := r.Rerank(context.Background(), "q", candidates("a", "b")); err != nil {
		t.Fatalf("Rerank: %v", err)
	}

	if got := r.RequestsTotal(); got != 2 {
		t.Fatalf("requests_total = %d, want 2", got)
	}
	if got := r.FailuresTotal(); got != 0 {
		t.Fatalf("failures_total = %d, want 0", got)
	}
	if got := r.LatencyMsTotal(); got <= 0 {
		t.Fatalf("latency_ms_total = %d, want > 0", got)
	}

	srv.status = 500
	srv.body = `{"detail": "boom"}`
	if _, err := r.Rerank(context.Background(), "q", candidates("a")); err == nil {
		t.Fatal("expected failure")
	}
	if got := r.FailuresTotal(); got != 1 {
		t.Fatalf("failures_total = %d, want 1", got)
	}
	if got := r.RequestsTotal(); got != 3 {
		t.Fatalf("requests_total = %d, want 3", got)
	}
}
