package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type standaloneReq struct {
	method, path, query string
	body                map[string]any
}

func standaloneServer(t *testing.T) (*httptest.Server, *[]standaloneReq) {
	t.Helper()
	var mu sync.Mutex
	var reqs []standaloneReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		reqs = append(reqs, standaloneReq{r.Method, r.URL.Path, r.URL.RawQuery, body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/system/server-address":
			_ = json.NewEncoder(w).Encode(apiclient.ServerAddress{IPv4: "203.0.113.4", Source: "detected", BehindNAT: true, Domain: r.URL.Query().Get("domain"),
				Records: []apiclient.DNSRecordSuggestion{{Type: "A", Name: r.URL.Query().Get("domain"), Value: "203.0.113.4"}}})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]apiclient.StandaloneDomain{{Domain: "test.example.com", TargetURL: "http://127.0.0.1:3000", HostHeader: "preserve", WebSocket: true}})
		default:
			_ = json.NewEncoder(w).Encode(apiclient.StandaloneDomain{Domain: "test.example.com", TargetURL: "http://127.0.0.1:3000"})
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func TestRun_DomainsStandalone(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantReq  string
		wantBody map[string]any
		absent   []string
		wantOut  string
	}{
		{
			name:     "add",
			args:     []string{"add", "test.example.com", "--target", "127.0.0.1:3000", "--confirm", "--strip-prefix", "/app"},
			wantReq:  "POST /api/v1/domains/standalone",
			wantBody: map[string]any{"domain": "test.example.com", "target": "127.0.0.1:3000", "confirm": true, "strip_prefix": "/app"},
			absent:   []string{"websocket", "upstream_tls"},
			wantOut:  "A  test.example.com  203.0.113.4",
		},
		{
			name:     "update sends only set flags",
			args:     []string{"update", "test.example.com", "--no-websocket", "--read-timeout", "60"},
			wantReq:  "PUT /api/v1/domains/standalone/test.example.com",
			wantBody: map[string]any{"websocket": false, "read_timeout_seconds": float64(60)},
			absent:   []string{"target", "domain"},
			wantOut:  "test.example.com -> http://127.0.0.1:3000",
		},
		{name: "list", args: []string{"list"}, wantReq: "GET /api/v1/domains/standalone", wantOut: "http://127.0.0.1:3000"},
		{name: "remove", args: []string{"remove", "test.example.com"}, wantReq: "DELETE /api/v1/domains/standalone/test.example.com", wantOut: "removed standalone domain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, reqs := standaloneServer(t)
			var stdout, stderr bytes.Buffer
			args := append(append([]string{"domains", "standalone"}, tt.args...), "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
				t.Fatalf("exit = %d stderr=%q", got, stderr.String())
			}
			first := (*reqs)[0]
			if first.method+" "+first.path != tt.wantReq {
				t.Fatalf("request = %s %s, want %s", first.method, first.path, tt.wantReq)
			}
			for k, v := range tt.wantBody {
				if first.body[k] != v {
					t.Errorf("body[%s] = %v, want %v (body %v)", k, first.body[k], v, first.body)
				}
			}
			for _, k := range tt.absent {
				if _, ok := first.body[k]; ok {
					t.Errorf("body has %s, want it omitted (body %v)", k, first.body)
				}
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

func TestRun_DomainsStandaloneAddNeedsTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "standalone", "add", "x.example.com"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want usage", got)
	}
	if !strings.Contains(stderr.String(), "--target is required") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRun_SystemAddress(t *testing.T) {
	srv, reqs := standaloneServer(t)
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"system", "address", "--domain", "test.example.com", "--ipv4", "203.0.113.4", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d stderr=%q", got, stderr.String())
	}
	if len(*reqs) != 2 || (*reqs)[0].method != http.MethodPut || (*reqs)[0].body["ipv4"] != "203.0.113.4" || (*reqs)[1].query != "domain=test.example.com" {
		t.Fatalf("requests = %+v", *reqs)
	}
	for _, want := range []string{"IPv4:   203.0.113.4", "behind NAT", "A  test.example.com  203.0.113.4"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if got := run("levelrail-cli-test", []string{"system", "address", "--clear", "--ipv4", "1.2.3.4"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Errorf("--clear with --ipv4 exit = %d, want usage", got)
	}
}
