package loadbalancer

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func exportCases() map[string]ExportInput {
	return map[string]ExportInput{
		"minimal": {Service: "web", Port: 3000, Domains: []string{"app.example.com"}, Config: Config{}},
		"full": {
			Service: "api", Port: 8080, Domains: []string{"api.example.com", "api2.example.com"},
			Upstreams: []string{"10.0.0.1:8080", "10.0.0.2:8080"},
			Config: Config{
				Algorithm:      AlgoLeastConn,
				ActiveHealth:   &ActiveHealth{Path: "/healthz", Interval: "10s", Timeout: "3s", Passes: 3, Fails: 2, ExpectStatus: 204},
				PassiveHealth:  &PassiveHealth{FailDuration: "45s", MaxFails: 3},
				Retries:        &Retries{Count: 2, TryDuration: "5s", TryInterval: "250ms"},
				SlowStart:      "60s",
				DrainTimeout:   "20s",
				RequestTimeout: "30s",
				RateLimit:      &RateLimit{RPS: 20, Burst: 50},
				UpstreamTLS:    &UpstreamTLS{ServerName: "api.internal"},
			},
		},
		"sticky": {Service: "shop cart", Port: 80, Config: Config{Algorithm: AlgoCookie, CookieName: "sid"}},
		"weighted": {
			Service: "canary", Port: 9000, Domains: []string{"c.example.com"},
			Upstreams: []string{"10.0.1.1:9000", "10.0.1.2:9000"},
			Config:    Config{Algorithm: AlgoWeighted, Weights: []int{9, 1}, SlowStart: "30s"},
		},
		"iphash": {Service: "ws", Port: 4000, Config: Config{Algorithm: AlgoIPHash}},
	}
}

func TestExportGolden(t *testing.T) {
	for caseName, in := range exportCases() {
		for _, format := range ExportFormats() {
			t.Run(format+"/"+caseName, func(t *testing.T) {
				art, err := Export(format, in)
				if err != nil {
					t.Fatalf("Export() error: %v", err)
				}
				out := art.Body
				if len(art.Warnings) > 0 {
					out += "\n# warnings\n# " + strings.Join(art.Warnings, "\n# ") + "\n"
				}
				path := filepath.Join("testdata", "golden", format+"_"+caseName+".golden")
				if *update {
					if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
				if err != nil {
					t.Fatalf("read golden (run with -update): %v", err)
				}
				if string(want) != out {
					t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", path, out, want)
				}
			})
		}
	}
}

func TestExportErrors(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		in      ExportInput
		wantErr string
	}{
		{"unknown format", "pulumi", ExportInput{Service: "a"}, "unknown format"},
		{"no service", FormatCDK, ExportInput{}, "service name"},
		{"invalid config", FormatCDK, ExportInput{Service: "a", Config: Config{Algorithm: "bad"}}, "algorithm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Export(tt.format, tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Export() = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	for in, want := range map[string]string{"Shop Cart": "shop-cart", "--x--": "x", "": "service", "a_b.c": "a-b-c"} {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
