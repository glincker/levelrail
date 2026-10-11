package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
)

type advisorFake struct {
	requests []string
	bodies   []string
}

func (f *advisorFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(strings.Builder)
		if r.Body != nil {
			b := make([]byte, 512)
			n, _ := r.Body.Read(b)
			buf.Write(b[:n])
		}
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		f.bodies = append(f.bodies, buf.String())
		w.Header().Set("Content-Type", "application/json")
		action := &resourceadvisor.Action{Type: resourceadvisor.ActionSetMemoryLimit, Value: 256 * 1024 * 1024}
		switch {
		case r.URL.Path == "/api/v1/resource-advisor/findings":
			_ = json.NewEncoder(w).Encode(apiclient.AdvisorFindings{Findings: []apiclient.AdvisorFinding{{
				Finding:   resourceadvisor.Finding{ID: "memory_limit_missing:app:web", Kind: resourceadvisor.KindMemoryLimitMissing, Severity: resourceadvisor.SeveritySuggest, Title: "Set a memory limit on web", Why: "because", Action: action},
				Appliable: true,
			}}})
		case r.URL.Path == "/api/v1/resource-advisor/summary":
			_, _ = w.Write([]byte(`{"total":2,"by_severity":{"suggest":2},"actionable":1,"saving_memory_bytes":0,"top_savings":[]}`))
		case strings.HasSuffix(r.URL.Path, "/apply"):
			confirm := strings.Contains(buf.String(), `"confirm":true`)
			_ = json.NewEncoder(w).Encode(apiclient.AdvisorApplyResult{FindingID: "memory_limit_missing:app:web", Resource: resourceadvisor.Ref{Kind: "app", Name: "web"}, Action: action,
				After: apiclient.AdvisorSettings{MemoryBytes: action.Value}, ConfirmRequired: !confirm, Applied: confirm})
		case strings.HasSuffix(r.URL.Path, "/snooze"):
			_, _ = w.Write([]byte(`{"id":"x","state":"snoozed","snoozed_until":"2026-10-18T00:00:00Z"}`))
		case strings.HasSuffix(r.URL.Path, "/dismiss"):
			_, _ = w.Write([]byte(`{"id":"x","state":"dismissed"}`))
		case r.URL.Path == "/api/v1/settings/beta-features/resource-advisor":
			_, _ = w.Write([]byte(`{"feature":"resource-advisor","enabled":true,"source":"settings"}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRun_Advisor(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantOut    string
		wantReq    string
		wantInBody string
	}{
		{"findings with filters", []string{"advisor", "findings", "--resource", "app:web", "--severity", "suggest", "--all"}, "no limit -> 256 MiB", "GET /api/v1/resource-advisor/findings?include=hidden&resource=app%3Aweb&severity=suggest", ""},
		{"summary", []string{"advisor", "summary"}, "findings: 2", "GET /api/v1/resource-advisor/summary", ""},
		{"apply previews by default", []string{"advisor", "apply", "memory_limit_missing:app:web"}, "preview only, nothing changed", "POST /api/v1/resource-advisor/findings/memory_limit_missing:app:web/apply", `"confirm":false`},
		{"apply with yes confirms", []string{"advisor", "apply", "memory_limit_missing:app:web", "--yes"}, "applied", "POST /api/v1/resource-advisor/findings/memory_limit_missing:app:web/apply", `"confirm":true`},
		{"snooze sends the duration", []string{"advisor", "snooze", "x", "--for", "2d"}, "snoozed x until", "POST /api/v1/resource-advisor/findings/x/snooze", `"duration":"2d"`},
		{"dismiss", []string{"advisor", "dismiss", "x"}, "dismissed x", "POST /api/v1/resource-advisor/findings/x/dismiss", ""},
		{"enable switches the beta", []string{"advisor", "enable"}, "is on", "PUT /api/v1/settings/beta-features/resource-advisor", `"enabled":true`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &advisorFake{}
			srv := httptest.NewServer(f.handler())
			defer srv.Close()
			out, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if !strings.Contains(out, tc.wantOut) {
				t.Fatalf("output %q missing %q", out, tc.wantOut)
			}
			if len(f.requests) != 1 || f.requests[0] != tc.wantReq {
				t.Fatalf("requests = %v, want %s", f.requests, tc.wantReq)
			}
			if tc.wantInBody != "" && !strings.Contains(f.bodies[0], tc.wantInBody) {
				t.Fatalf("body %q missing %q", f.bodies[0], tc.wantInBody)
			}
		})
	}
}

func TestRun_AdvisorUsage(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := runAdvisor("cli", nil, &stdout, &stderr, func(string) (string, bool) { return "", false }); code != exitUsage || !strings.Contains(stderr.String(), "beta") {
		t.Fatalf("no args = %d, %q", code, stderr.String())
	}
	stderr.Reset()
	if code := runAdvisor("cli", []string{"bogus"}, &stdout, &stderr, func(string) (string, bool) { return "", false }); code != exitUsage {
		t.Fatalf("unknown verb = %d", code)
	}
}
