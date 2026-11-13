package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

type idemRecorder struct {
	mu      sync.Mutex
	keys    []string
	failOne bool
}

func (rec *idemRecorder) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.keys = append(rec.keys, r.Header.Get("Idempotency-Key"))
		first := len(rec.keys) == 1 && rec.failOne
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if first {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "busy"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"stack": "shop", "revision": map[string]any{"number": 4, "status": "applied", "plan_hash": "h1", "actor": "ci", "created_at": "2026-10-25T10:00:00Z"}, "result": map[string]any{"applied": 1}})
	})
}

func TestStackApply_SendsRandomIdempotencyKeyHeader(t *testing.T) {
	rec := &idemRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	args := []string{"stack", "apply", "shop", "-f", stackTestDir(t), "--yes", "--api-url", srv.URL}
	if code, out, errOut := runIaC(args, nil); code != exitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	runIaC(args, nil)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(rec.keys[0]) {
		t.Fatalf("key %q is not 128 bit hex", rec.keys[0])
	}
	if len(rec.keys) != 2 || rec.keys[0] == rec.keys[1] {
		t.Fatalf("each invocation needs its own key: %v", rec.keys)
	}
}

func TestStackApply_FlagKeyAndRetryReuseSameKey(t *testing.T) {
	rec := &idemRecorder{failOne: true}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	code, out, errOut := runIaC([]string{"stack", "apply", "shop", "-f", stackTestDir(t), "--yes", "--idempotency-key", "k-1", "--api-url", srv.URL}, nil)
	if code != exitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if len(rec.keys) != 2 || rec.keys[0] != "k-1" || rec.keys[1] != "k-1" {
		t.Fatalf("retry must reuse the key: %v", rec.keys)
	}
}

func TestStackRollbackApprove_SendIdempotencyKeyHeader(t *testing.T) {
	for _, args := range [][]string{{"stack", "rollback", "shop", "2", "--yes"}, {"stack", "approve", "shop", "4"}} {
		rec := &idemRecorder{}
		srv := httptest.NewServer(rec.handler())
		code, out, errOut := runIaC(append(args, "--api-url", srv.URL), nil)
		srv.Close()
		if code != exitOK || len(rec.keys) != 1 || len(rec.keys[0]) != 32 {
			t.Fatalf("%s: exit %d keys %v\n%s%s", strings.Join(args, " "), code, rec.keys, out, errOut)
		}
	}
}
