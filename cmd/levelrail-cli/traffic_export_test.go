package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type texCall struct {
	method, path string
	body         apiclient.TrafficExportSink
}

func texServer(t *testing.T, respond func(c texCall) (int, any)) (*httptest.Server, *[]texCall) {
	t.Helper()
	var calls []texCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := texCall{method: r.Method, path: r.URL.Path}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&c.body)
		}
		calls = append(calls, c)
		code, out := respond(c)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if out != nil {
			_ = json.NewEncoder(w).Encode(out)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func runTex(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = runTrafficExport("levelrail-cli-test", args, strings.NewReader(stdin), &out, &errBuf, envMap())
	return code, out.String(), errBuf.String()
}

func TestTrafficExportAddReadsSecretsFromStdinNeverArgv(t *testing.T) {
	enabled := true
	srv, calls := texServer(t, func(c texCall) (int, any) {
		return http.StatusCreated, apiclient.TrafficExportSink{ID: "tex_1", Name: c.body.Name, Kind: c.body.Kind, Enabled: &enabled, URLSet: c.body.URL != "", AuthHeaderSet: c.body.AuthHeader != "", TargetHost: "hooks.example.com"}
	})
	code, stdout, stderr := runTex(t, "https://hooks.example.com/in/SECRET\nBearer abc\n", "sinks", "add", "--kind", "webhook", "--name", "Warehouse", "--secret-stdin", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	c := (*calls)[0]
	if c.method != http.MethodPost || c.path != "/api/v1/settings/traffic-export/sinks" {
		t.Errorf("call = %s %s", c.method, c.path)
	}
	if c.body.URL != "https://hooks.example.com/in/SECRET" || c.body.AuthHeader != "Bearer abc" || c.body.Kind != "webhook" {
		t.Errorf("body = %+v", c.body)
	}
	if strings.Contains(stdout+stderr, "SECRET") || strings.Contains(stdout+stderr, "Bearer abc") {
		t.Errorf("a credential reached the output: %q %q", stdout, stderr)
	}
	if strings.Contains(stderr, "visitor identity") {
		t.Errorf("no identity warning expected without fields: %q", stderr)
	}
}

func TestTrafficExportAddPostHogToken(t *testing.T) {
	srv, calls := texServer(t, func(c texCall) (int, any) {
		return http.StatusCreated, apiclient.TrafficExportSink{ID: "tex_1", Name: c.body.Name, Kind: c.body.Kind, TokenSet: true}
	})
	code, _, stderr := runTex(t, "phc_tok\n", "sinks", "add", "--kind", "posthog", "--name", "PH", "--host", "eu", "--secret-stdin", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if b := (*calls)[0].body; b.Token != "phc_tok" || b.Host != "eu" || b.URL != "" {
		t.Errorf("body = %+v", b)
	}
}

func TestTrafficExportFieldsWarnAboutIdentity(t *testing.T) {
	srv, calls := texServer(t, func(texCall) (int, any) {
		return http.StatusCreated, apiclient.TrafficExportSink{ID: "tex_1", Name: "n", Kind: "webhook"}
	})
	code, _, stderr := runTex(t, "https://example.com/x\n", "sinks", "add", "--kind", "webhook", "--name", "n", "--fields", "client_ip, user_agent", "--ip-truncate=false", "--secret-stdin", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	b := (*calls)[0].body
	if len(b.Fields) != 2 || b.Fields[0] != "client_ip" || b.Fields[1] != "user_agent" || b.IPTruncate == nil || *b.IPTruncate {
		t.Errorf("body = %+v", b)
	}
	if !strings.Contains(stderr, "visitor identity") {
		t.Errorf("stderr = %q, want an identity notice", stderr)
	}
}

func TestTrafficExportSetOnlyChangesPassedFlagsAndKeepsSecrets(t *testing.T) {
	enabled := true
	srv, calls := texServer(t, func(texCall) (int, any) {
		return http.StatusOK, apiclient.TrafficExportSink{ID: "tex_1", Name: "n", Kind: "webhook", Enabled: &enabled, Fields: []string{"client_ip"}, URLSet: true}
	})
	code, _, stderr := runTex(t, "", "sinks", "set", "tex_1", "--enabled=false", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	put := (*calls)[1]
	if put.method != http.MethodPut || put.path != "/api/v1/settings/traffic-export/sinks/tex_1" {
		t.Fatalf("call = %s %s", put.method, put.path)
	}
	if put.body.Enabled == nil || *put.body.Enabled || put.body.URL != "" || put.body.Token != "" || len(put.body.Fields) != 1 {
		t.Errorf("body = %+v, want enabled=false, fields kept, no credential sent", put.body)
	}
}

func TestTrafficExportRmTestAndStatus(t *testing.T) {
	srv, calls := texServer(t, func(c texCall) (int, any) {
		switch {
		case c.method == http.MethodDelete:
			return http.StatusNoContent, nil
		case strings.HasSuffix(c.path, "/test"):
			return http.StatusOK, apiclient.TrafficExportTestResult{OK: false, ErrorKind: "auth"}
		}
		return http.StatusOK, apiclient.TrafficExportStatus{Sinks: []apiclient.TrafficExportSinkStatus{{ID: "tex_1", Name: "PH", Kind: "posthog", Enabled: true, Active: true, Dropped: 7, Queued: 2, LastErrorKind: "timeout"}}}
	})
	if code, out, _ := runTex(t, "", "sinks", "rm", "tex_1", "--api-url", srv.URL); code != exitOK || !strings.Contains(out, "removed") {
		t.Errorf("rm = %d %q", code, out)
	}
	code, out, _ := runTex(t, "", "sinks", "test", "tex_1", "--api-url", srv.URL)
	if code != exitValidation || !strings.Contains(out, "failed: auth") {
		t.Errorf("test = %d %q, want a failing exit with the error kind", code, out)
	}
	code, out, _ = runTex(t, "", "sinks", "status", "--api-url", srv.URL)
	if code != exitOK || !strings.Contains(out, "PH") || !strings.Contains(out, "timeout") || !strings.Contains(out, "active") {
		t.Errorf("status = %d %q", code, out)
	}
	if len(*calls) != 3 {
		t.Errorf("calls = %d", len(*calls))
	}
}

func TestTrafficExportUsageAndUnknown(t *testing.T) {
	if code, _, _ := runTex(t, "", "sinks"); code != exitUsage {
		t.Errorf("bare sinks = %d", code)
	}
	if code, out, _ := runTex(t, "", "-h"); code != exitOK || !strings.Contains(out, "Credentials are never passed as arguments") {
		t.Errorf("help = %d %q", code, out)
	}
	if code, _, _ := runTex(t, "", "sinks", "frobnicate"); code != exitUsage {
		t.Errorf("unknown = %d", code)
	}
	if code, _, _ := runTex(t, "", "sinks", "add", "--api-url", "http://127.0.0.1:1"); code != exitValidation {
		t.Errorf("add without kind and name = %d", code)
	}
}
