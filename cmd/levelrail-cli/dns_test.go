package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type dnsCall struct {
	method, path, query string
	body                map[string]any
}

func dnsTestServer(t *testing.T, calls *[]dnsCall, reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := dnsCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &c.body)
		}
		*calls = append(*calls, c)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_DNSCommands(t *testing.T) {
	dir := t.TempDir()
	zoneFile := filepath.Join(dir, "example.com.zone")
	if err := os.WriteFile(zoneFile, []byte("www 300 IN A 1.2.3.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   map[string]any
	}{
		{name: "zones list", args: []string{"dns", "zones", "list", "--provider", "route53"}, wantExit: exitOK, wantMethod: "GET", wantPath: "/api/v1/dns/zones", wantQuery: "provider=route53"},
		{name: "zones create", args: []string{"dns", "zones", "create", "example.com"}, wantExit: exitOK, wantMethod: "POST", wantPath: "/api/v1/dns/zones", wantBody: map[string]any{"name": "example.com"}},
		{name: "zones delete needs confirm", args: []string{"dns", "zones", "delete", "example.com"}, wantExit: exitUsage},
		{name: "zones delete", args: []string{"dns", "zones", "delete", "example.com", "--confirm", "example.com", "--force"}, wantExit: exitOK, wantMethod: "DELETE", wantPath: "/api/v1/dns/zones/example.com", wantBody: map[string]any{"confirm": "example.com", "force": true}},
		{name: "zones verify", args: []string{"dns", "zones", "verify", "example.com"}, wantExit: exitOK, wantMethod: "GET", wantPath: "/api/v1/dns/zones/example.com/delegation"},
		{name: "zones nameservers", args: []string{"dns", "zones", "nameservers", "z1"}, wantExit: exitOK, wantMethod: "GET", wantPath: "/api/v1/dns/zones/z1/nameservers"},
		{name: "records list filters", args: []string{"dns", "records", "list", "example.com", "--type", "MX", "--search", "mail"}, wantExit: exitOK, wantMethod: "GET", wantPath: "/api/v1/dns/zones/example.com/records", wantQuery: "q=mail&type=MX"},
		{name: "records add multi value", args: []string{"dns", "records", "add", "example.com", "--name", "www", "--type", "a", "--value", "1.1.1.1", "--value", "2.2.2.2", "--proxied"}, wantExit: exitOK, wantMethod: "POST", wantPath: "/api/v1/dns/zones/example.com/records", wantBody: map[string]any{"name": "www", "type": "A", "proxied": true}},
		{name: "records add weighted", args: []string{"dns", "records", "add", "example.com", "--name", "lb", "--type", "A", "--value", "1.1.1.1", "--routing", "weighted", "--set-id", "a", "--weight", "0"}, wantExit: exitOK, wantMethod: "POST", wantPath: "/api/v1/dns/zones/example.com/records", wantBody: map[string]any{"weight": float64(0), "set_identifier": "a"}},
		{name: "records add needs value", args: []string{"dns", "records", "add", "example.com", "--type", "A"}, wantExit: exitUsage},
		{name: "records update rename", args: []string{"dns", "records", "update", "example.com", "--name", "a", "--type", "A", "--value", "9.9.9.9", "--new-name", "b"}, wantExit: exitOK, wantMethod: "PUT", wantPath: "/api/v1/dns/zones/example.com/records"},
		{name: "records delete", args: []string{"dns", "records", "delete", "example.com", "--name", "www", "--type", "A"}, wantExit: exitOK, wantMethod: "DELETE", wantPath: "/api/v1/dns/zones/example.com/records", wantQuery: "name=www&type=A"},
		{name: "records import preview", args: []string{"dns", "records", "import", "example.com", "--file", zoneFile}, wantExit: exitOK, wantMethod: "POST", wantPath: "/api/v1/dns/zones/example.com/records/import", wantBody: map[string]any{"format": "bind"}},
		{name: "records import needs file", args: []string{"dns", "records", "import", "example.com"}, wantExit: exitUsage},
		{name: "records export", args: []string{"dns", "records", "export", "example.com", "--format", "json"}, wantExit: exitOK, wantMethod: "GET", wantPath: "/api/v1/dns/zones/example.com/records/export", wantQuery: "format=json"},
		{name: "records template", args: []string{"dns", "records", "template", "example.com", "google-workspace", "--param", "policy=none", "--apply"}, wantExit: exitOK, wantMethod: "POST", wantPath: "/api/v1/dns/zones/example.com/templates/google-workspace", wantBody: map[string]any{"apply": true}},
		{name: "check", args: []string{"dns", "check", "www.example.com", "--type", "aaaa"}, wantExit: exitOK, wantMethod: "GET", wantPath: "/api/v1/dns/check", wantQuery: "name=www.example.com&type=AAAA"},
		{name: "health checks create", args: []string{"dns", "health-checks", "create", "--fqdn", "app.example.com", "--path", "/healthz"}, wantExit: exitOK, wantMethod: "POST", wantPath: "/api/v1/dns/health-checks", wantBody: map[string]any{"type": "HTTPS", "fqdn": "app.example.com"}},
		{name: "unknown", args: []string{"dns", "nope"}, wantExit: exitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []dnsCall
			srv := dnsTestServer(t, &calls, `{}`)
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", append(tt.args, "--api-url", srv.URL), &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tt.wantExit, stderr.String())
			}
			if tt.wantMethod == "" {
				return
			}
			if len(calls) != 1 {
				t.Fatalf("calls = %+v", calls)
			}
			c := calls[0]
			if c.method != tt.wantMethod || c.path != tt.wantPath || (tt.wantQuery != "" && c.query != tt.wantQuery) {
				t.Errorf("request = %s %s?%s, want %s %s?%s", c.method, c.path, c.query, tt.wantMethod, tt.wantPath, tt.wantQuery)
			}
			for k, v := range tt.wantBody {
				if c.body[k] != v {
					t.Errorf("body[%s] = %v, want %v (body %v)", k, c.body[k], v, c.body)
				}
			}
		})
	}
}

func TestDNSImportFormat(t *testing.T) {
	tests := []struct{ format, file, content, want string }{
		{"", "a.zone", "www IN A 1.1.1.1", "bind"},
		{"", "a.json", "", "json"},
		{"", "-", `{"records":[]}`, "json"},
		{"BIND", "a.json", "", "bind"},
	}
	for _, tt := range tests {
		if got := dnsImportFormat(tt.format, tt.file, []byte(tt.content)); got != tt.want {
			t.Errorf("dnsImportFormat(%q,%q) = %q, want %q", tt.format, tt.file, got, tt.want)
		}
	}
	if !strings.Contains(dnsUsage("x"), "dns zones verify") {
		t.Error("usage must list verify")
	}
}
