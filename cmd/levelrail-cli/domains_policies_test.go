package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// policyRecorder answers every GET with getBody and records the last write.
type policyRecorder struct {
	getBody string
	method  string
	path    string
	query   string
	body    string
}

func (p *policyRecorder) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && p.getBody != "" {
			_, _ = w.Write([]byte(p.getBody))
			return
		}
		b, _ := io.ReadAll(r.Body)
		p.method, p.path, p.query, p.body = r.Method, r.URL.Path, r.URL.RawQuery, string(b)
		_, _ = w.Write([]byte(`{"domain":"a.example.com"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_DomainsPolicies(t *testing.T) {
	dir := t.TempDir()
	headersFile := filepath.Join(dir, "headers.json")
	if err := os.WriteFile(headersFile, []byte(`{"rules":[{"side":"request","op":"set","name":"X-Env","value":"prod"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	const base = "/api/v1/apps/web/domains/a.example.com/"
	tests := []struct {
		name       string
		getBody    string
		args       []string
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   []string
	}{
		{name: "headers set file", args: []string{"headers", "set", "web", "a.example.com", "--file", headersFile}, wantMethod: "PUT", wantPath: base + "headers", wantBody: []string{`"name":"X-Env"`}},
		{name: "headers add appends", getBody: `{"spec":{"rules":[{"side":"response","op":"remove","name":"X-Old"}]}}`, args: []string{"headers", "add", "web", "a.example.com", "--name", "X-New", "--value", "1"}, wantMethod: "PUT", wantPath: base + "headers", wantBody: []string{`"X-Old"`, `"name":"X-New"`, `"side":"response"`}},
		{name: "headers preset cors", getBody: `{"spec":{"rules":[]}}`, args: []string{"headers", "preset", "web", "a.example.com", "cors", "--origin", "https://app.example.com", "--credentials"}, wantMethod: "PUT", wantPath: base + "headers", wantBody: []string{`"origins":["https://app.example.com"]`, `"credentials":true`, `"preflight":true`}},
		{name: "headers clear", args: []string{"headers", "clear", "web", "a.example.com"}, wantMethod: "DELETE", wantPath: base + "headers"},
		{name: "forwarders add app", getBody: `{"spec":{"rules":[]}}`, args: []string{"forwarders", "add", "web", "a.example.com", "--path", "/api", "--app", "api", "--strip-prefix", "--methods", "get,post"}, wantMethod: "PUT", wantPath: base + "forwarders", wantBody: []string{`"action":"app"`, `"app":"api"`, `"strip_prefix":true`, `"methods":["GET","POST"]`}},
		{name: "forwarders add url no websocket", getBody: `{"spec":{"rules":[]}}`, args: []string{"forwarders", "add", "web", "a.example.com", "--path", "/ext", "--url", "https://api.example.org", "--no-websocket"}, wantMethod: "PUT", wantPath: base + "forwarders", wantBody: []string{`"action":"url"`, `"websocket":false`}},
		{name: "forwarders remove", getBody: `{"spec":{"rules":[{"match":{"kind":"prefix","path":"/a"},"action":"app","app":"x"},{"match":{"kind":"prefix","path":"/b"},"action":"app","app":"y"}]}}`, args: []string{"forwarders", "remove", "web", "a.example.com", "1"}, wantMethod: "PUT", wantPath: base + "forwarders", wantBody: []string{`"path":"/b"`}},
		{name: "geo set", args: []string{"geo", "set", "web", "a.example.com", "--mode", "deny", "--countries", "ru,kp", "--exempt", "203.0.113.7"}, wantMethod: "PUT", wantPath: base + "geo", wantBody: []string{`"mode":"deny"`, `"countries":["RU","KP"]`, `"exempt":["203.0.113.7"]`, `"action":"block"`}},
		{name: "cache add", getBody: `{"spec":{"enabled":false,"rules":[]}}`, args: []string{"cache", "add", "web", "a.example.com", "--path", "/static", "--ttl", "300", "--vary", "Accept-Language"}, wantMethod: "PUT", wantPath: base + "cache", wantBody: []string{`"enabled":true`, `"ttl_seconds":300`, `"vary":["Accept-Language"]`}},
		{name: "cache purge prefix", args: []string{"cache", "purge", "web", "a.example.com", "--prefix", "/static"}, wantMethod: "POST", wantPath: base + "cache/purge", wantBody: []string{`"scope":"prefix"`, `"value":"/static"`}},
		{name: "cache purge all", args: []string{"cache", "purge", "web", "a.example.com", "--all"}, wantMethod: "POST", wantPath: base + "cache/purge", wantBody: []string{`"scope":"all"`}},
		{name: "redirects force https", getBody: `{"settings":{"force_https":false,"trailing_slash":"add"}}`, args: []string{"redirects", "force-https", "web", "a.example.com", "on", "--status", "301"}, wantMethod: "PUT", wantPath: base + "redirects", wantBody: []string{`"force_https":true`, `"force_https_status":301`, `"trailing_slash":"add"`}},
		{name: "redirects canonical", args: []string{"redirects", "canonical", "web", "a.example.com", "www-to-apex", "--no-attach"}, wantMethod: "POST", wantPath: base + "redirects/canonical", wantBody: []string{`"preset":"www-to-apex"`, `"attach":false`}},
		{name: "redirects alias", args: []string{"redirects", "alias", "web", "a.example.com", "--alias", "b.example.com", "--alias", "c.example.com", "--status", "308"}, wantMethod: "PUT", wantPath: base + "redirects/aliases", wantBody: []string{`"aliases":["b.example.com","c.example.com"]`, `"status_code":308`}},
		{name: "redirect set preset", args: []string{"redirect", "set", "web", "a.example.com", "--preset", "apex-to-www"}, wantMethod: "POST", wantPath: base + "redirects/canonical", wantBody: []string{`"preset":"apex-to-www"`}},
		{name: "redirect set status 308", args: []string{"redirect", "set", "web", "a.example.com", "--target", "https://b.example.com", "--status", "308"}, wantMethod: "PUT", wantPath: base + "redirect", wantBody: []string{`"status_code":308`}},
		{name: "ports restrict", args: []string{"ports", "restrict", "web", "a.example.com", "5432", "--source", "10.0.0.0/8"}, wantMethod: "PUT", wantPath: base + "ports/5432/restrict", wantBody: []string{`"sources":["10.0.0.0/8"]`}},
		{name: "ports unrestrict", args: []string{"ports", "unrestrict", "web", "a.example.com", "5432"}, wantMethod: "DELETE", wantPath: base + "ports/5432/restrict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &policyRecorder{getBody: tt.getBody}
			srv := rec.server(t)
			args := append(append([]string{"domains"}, tt.args...), "--api-url", srv.URL)
			runCLIExpectOK(t, args)
			if rec.method != tt.wantMethod || rec.path != tt.wantPath || rec.query != tt.wantQuery {
				t.Errorf("request = %s %s?%s, want %s %s?%s", rec.method, rec.path, rec.query, tt.wantMethod, tt.wantPath, tt.wantQuery)
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(rec.body, want) {
					t.Errorf("body = %s, want it to contain %s", rec.body, want)
				}
			}
		})
	}
}

func TestRun_DomainsPoliciesUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"forwarders two targets", []string{"forwarders", "add", "web", "a.example.com", "--path", "/x", "--app", "a", "--url", "https://b.example.org"}, "exactly one of --app, --url or --redirect"},
		{"forwarders no path", []string{"forwarders", "add", "web", "a.example.com", "--app", "a"}, "--path is required"},
		{"purge two scopes", []string{"cache", "purge", "web", "a.example.com", "--all", "--prefix", "/x"}, "exactly one of --url, --prefix or --all"},
		{"force https bad value", []string{"redirects", "force-https", "web", "a.example.com", "maybe"}, "on or off"},
		{"slash bad value", []string{"redirects", "slash", "web", "a.example.com", "sideways"}, "add, remove or off"},
		{"restrict no source", []string{"ports", "restrict", "web", "a.example.com", "5432"}, "--source"},
		{"missing domain", []string{"headers", "show", "web"}, "requires"},
		{"unknown verb", []string{"geo", "explode"}, "unknown domains geo subcommand"},
		{"preset and target", []string{"redirect", "set", "web", "a.example.com", "--preset", "www-to-apex", "--target", "https://x.example.com"}, "mutually exclusive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", append([]string{"domains"}, tt.args...), &stdout, &stderr, envMap())
			if got != exitUsage {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestRun_DomainsGeoLookup(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte(`{"status":{"active":true,"sources":["mmdb"]},"ip":"81.2.69.1","country":"GB","source":"mmdb"}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "geo", "lookup", "81.2.69.1", "--api-url", srv.URL})
	if gotQuery != "/api/v1/system/geoip?ip=81.2.69.1" {
		t.Errorf("request = %s", gotQuery)
	}
	if !strings.Contains(stdout, "81.2.69.1: GB") {
		t.Errorf("stdout = %q", stdout)
	}
}
