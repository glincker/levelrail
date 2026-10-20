package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubPinger struct{ err error }

func (s stubPinger) Ping(context.Context) error { return s.err }

func TestHandleReadyz(t *testing.T) {
	ok := func(context.Context) error { return nil }
	bad := func(context.Context) error { return errors.New("secret internal detail") }
	up := func() bool { return true }
	tests := []struct {
		name      string
		probes    ReadinessProbes
		docker    DockerPinger
		wantCode  int
		wantReady bool
		wantCheck map[string]string
	}{
		{"all ok", ReadinessProbes{ok, ok, up}, stubPinger{}, 200, true,
			map[string]string{"database": "ok", "migrations": "ok", "reconcile_engine": "ok", "docker": "ok"}},
		{"docker down is degraded", ReadinessProbes{ok, ok, up}, stubPinger{errors.New("x")}, 200, true,
			map[string]string{"docker": "degraded"}},
		{"database down", ReadinessProbes{bad, ok, up}, nil, 503, false,
			map[string]string{"database": "failing"}},
		{"migrations pending", ReadinessProbes{ok, bad, up}, nil, 503, false,
			map[string]string{"migrations": "failing"}},
		{"engine not started", ReadinessProbes{ok, ok, func() bool { return false }}, nil, 503, false,
			map[string]string{"reconcile_engine": "failing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{readiness: tt.probes, dockerPinger: tt.docker}
			rec := httptest.NewRecorder()
			rt.handleReadyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			var got readyzResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Ready != tt.wantReady {
				t.Errorf("ready = %v, want %v", got.Ready, tt.wantReady)
			}
			seen := map[string]string{}
			for _, c := range got.Checks {
				seen[c.Name] = c.Status
			}
			for name, want := range tt.wantCheck {
				if seen[name] != want {
					t.Errorf("check %s = %q, want %q", name, seen[name], want)
				}
			}
			if strings.Contains(rec.Body.String(), "secret internal detail") {
				t.Errorf("response leaks error detail: %s", rec.Body.String())
			}
		})
	}
}
