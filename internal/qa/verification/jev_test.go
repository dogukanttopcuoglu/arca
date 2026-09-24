package verification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// jevServer is a configurable fake of the POST /v1/systemone contract,
// recording the request body for contract assertions.
type jevServer struct {
	status   int
	body     string
	delay    time.Duration
	requests atomic.Int64
	lastBody string
	lastAuth string
}

func (s *jevServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		s.lastAuth = r.Header.Get("Authorization")
		buf := new(strings.Builder)
		if _, err := io.Copy(buf, r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
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

func noulBody(support, contradict float64) string {
	return `{"answers":{"support":{"noul":` + jsonNumber(support) + `},"contradict":{"noul":` + jsonNumber(contradict) + `}}}`
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func TestHTTPJevChecker_RelationMapping(t *testing.T) {
	tt := []struct {
		name       string
		support    float64
		contradict float64
		wantScore  float64
		wantRel    string
	}{
		{"high support maps to entailed", 0.9, 0.05, 0.9, "entailed"},
		{"high contradict maps to contradicted", 0.2, 0.8, 0.2, "contradicted"},
		{"neither above threshold maps to neutral", 0.4, 0.4, 0.4, "neutral"},
		{"support meets threshold exactly maps to entailed", 0.7, 0.1, 0.7, "entailed"},
		{"support 0.69 stays neutral at default threshold", 0.69, 0.1, 0.69, "neutral"},
		{"support 0.71 maps to entailed at default threshold", 0.71, 0.1, 0.71, "entailed"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			srv := &jevServer{status: 200, body: noulBody(tc.support, tc.contradict)}
			ts := httptest.NewServer(srv.handler())
			defer ts.Close()

			// Zero threshold and timeout exercise the constructor defaults.
			c := NewHTTPJevChecker(ts.URL, "test-key", 0, 0)
			got, err := c.CheckEntailment(context.Background(), "claim", "source")
			if err != nil {
				t.Fatalf("CheckEntailment: %v", err)
			}
			if got.Score != tc.wantScore {
				t.Errorf("score = %v, want %v", got.Score, tc.wantScore)
			}
			if got.Relation != tc.wantRel {
				t.Errorf("relation = %q, want %q", got.Relation, tc.wantRel)
			}
			if c.RequestsTotal() != 1 {
				t.Errorf("requests = %d, want 1", c.RequestsTotal())
			}
			if c.FailuresTotal() != 0 || c.LastDegraded() {
				t.Errorf("happy path must not flag failures or degrade")
			}
		})
	}
}

func TestHTTPJevChecker_RequestContract(t *testing.T) {
	// The delay guarantees a non-zero millisecond latency so the atomic
	// latency counter is asserted deterministically.
	srv := &jevServer{status: 200, body: noulBody(0.9, 0.1), delay: time.Millisecond}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	c := NewHTTPJevChecker(ts.URL, "test-key", 5*time.Second, 0.7)
	source := strings.Repeat("é", 2500)
	for i := 0; i < 2; i++ {
		if _, err := c.CheckEntailment(context.Background(), "the claim text", source); err != nil {
			t.Fatalf("CheckEntailment: %v", err)
		}
	}

	if srv.requests.Load() != 2 {
		t.Fatalf("server saw %d requests, want 2", srv.requests.Load())
	}
	if srv.lastAuth != "Bearer test-key" {
		t.Errorf("authorization = %q, want Bearer test-key", srv.lastAuth)
	}

	var req struct {
		Model string `json:"model"`
		State struct {
			Claim  string `json:"claim"`
			Source string `json:"source"`
		} `json:"state"`
		Questions struct {
			Support struct {
				Type         string `json:"type"`
				Instructions string `json:"instructions"`
			} `json:"support"`
			Contradict struct {
				Type         string `json:"type"`
				Instructions string `json:"instructions"`
			} `json:"contradict"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(srv.lastBody), &req); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, srv.lastBody)
	}
	if req.Model != "jev-latest" {
		t.Errorf("model = %q, want jev-latest", req.Model)
	}
	if req.State.Claim != "the claim text" {
		t.Errorf("claim = %q", req.State.Claim)
	}
	if got := len([]rune(req.State.Source)); got != 2000 {
		t.Errorf("source clipped to %d runes, want 2000", got)
	}
	if req.Questions.Support.Type != "noul" || req.Questions.Contradict.Type != "noul" {
		t.Errorf("question types = %q/%q, want noul/noul", req.Questions.Support.Type, req.Questions.Contradict.Type)
	}
	if !strings.Contains(req.Questions.Support.Instructions, "support the claim") {
		t.Errorf("support instructions = %q", req.Questions.Support.Instructions)
	}
	if !strings.Contains(req.Questions.Contradict.Instructions, "contradict the claim") {
		t.Errorf("contradict instructions = %q", req.Questions.Contradict.Instructions)
	}

	if c.LatencyMsTotal() <= 0 {
		t.Errorf("latency_ms_total = %d, want > 0", c.LatencyMsTotal())
	}

	if c.RequestsTotal() != 2 {
		t.Errorf("requests = %d, want 2", c.RequestsTotal())
	}
}

func TestHTTPJevChecker_FailureMatrix(t *testing.T) {
	tt := []struct {
		name   string
		server *jevServer
	}{
		{"5xx response", &jevServer{status: 500, body: `{"detail": "boom"}`}},
		{"invalid JSON", &jevServer{status: 200, body: `not json`}},
		{"missing answers field", &jevServer{status: 200, body: `{"ok": true}`}},
		{"missing support inside answers", &jevServer{status: 200, body: `{"answers":{"support":{},"contradict":{"noul":0.5}}}`}},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.server.handler())
			defer ts.Close()

			c := NewHTTPJevChecker(ts.URL, "test-key", 5*time.Second, 0.7)
			got, err := c.CheckEntailment(context.Background(), "claim", "source")
			if err == nil {
				t.Fatalf("expected error, got %+v", got)
			}
			if c.FailuresTotal() != 1 {
				t.Errorf("failures = %d, want 1", c.FailuresTotal())
			}
			if !c.LastDegraded() {
				t.Error("last call must be marked degraded")
			}
		})
	}

	t.Run("timeout", func(t *testing.T) {
		srv := &jevServer{status: 200, body: noulBody(0.9, 0.1), delay: 200 * time.Millisecond}
		ts := httptest.NewServer(srv.handler())
		defer ts.Close()

		c := NewHTTPJevChecker(ts.URL, "test-key", 20*time.Millisecond, 0.7)
		if _, err := c.CheckEntailment(context.Background(), "claim", "source"); err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if c.FailuresTotal() != 1 || !c.LastDegraded() {
			t.Errorf("timeout must count a failure and degrade")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		c := NewHTTPJevChecker("http://127.0.0.1:1", "test-key", 2*time.Second, 0.7)
		if _, err := c.CheckEntailment(context.Background(), "claim", "source"); err == nil {
			t.Fatal("expected connection error, got nil")
		}
		if c.FailuresTotal() != 1 || !c.LastDegraded() {
			t.Errorf("connection failure must count a failure and degrade")
		}
	})
}

func TestHTTPJevChecker_DebugLine(t *testing.T) {
	srv := &jevServer{status: 200, body: noulBody(0.9, 0.1)}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	var buf strings.Builder
	old := debugWriter
	debugWriter = &buf
	defer func() { debugWriter = old }()

	c := NewHTTPJevChecker(ts.URL, "test-key", 5*time.Second, 0.7)
	if _, err := c.CheckEntailment(context.Background(), "claim", "source"); err != nil {
		t.Fatalf("CheckEntailment: %v", err)
	}
	line := buf.String()
	for _, want := range []string{"jev_verify: claim_len=5 source_len=6", "support=0.900", "contradict=0.100", "relation=entailed", "degraded=false"} {
		if !strings.Contains(line, want) {
			t.Fatalf("debug line missing %q: %q", want, line)
		}
	}
	if strings.Contains(line, "error=") {
		t.Fatalf("happy path must not carry error=: %q", line)
	}

	buf.Reset()
	srv.status = 500
	srv.body = `{"detail": "boom"}`
	if _, err := c.CheckEntailment(context.Background(), "claim", "source"); err == nil {
		t.Fatal("expected failure")
	}
	line = buf.String()
	for _, want := range []string{"jev_verify:", "degraded=true", "error="} {
		if !strings.Contains(line, want) {
			t.Fatalf("failure debug line missing %q: %q", want, line)
		}
	}
}
