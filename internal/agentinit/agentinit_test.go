package agentinit

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/kit/stackdetect"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update)", name, err)
	}
	if string(want) != got {
		t.Errorf("%s mismatch\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

var testNames = Names{Product: "Acme", CLI: "acme-cli", MCPBinary: "acme-mcp", ServerKey: "acme"}

func TestAppYAML_Golden(t *testing.T) {
	tests := []struct {
		file string
		st   stackdetect.Stack
	}{
		{"app_dockerfile.yaml", stackdetect.Stack{Build: "dockerfile", Path: "./Dockerfile", Port: 9000, Name: "web"}},
		{"app_railpack_node.yaml", stackdetect.Stack{Build: "railpack", Port: 3000, Name: "site"}},
		{"app_static.yaml", stackdetect.Stack{Build: "static", Path: "./", Name: "docs"}},
		{"app_compose.yaml", stackdetect.Stack{Build: "compose", Path: "./compose.yaml", Name: "stack"}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			got, err := AppYAML(tc.st)
			if err != nil {
				t.Fatalf("AppYAML: %v", err)
			}
			golden(t, tc.file, got)
		})
	}
}

func TestAppYAML_Errors(t *testing.T) {
	if _, err := AppYAML(stackdetect.Stack{Name: "x"}); err == nil {
		t.Error("want error when nothing detected")
	}
	if _, err := AppYAML(stackdetect.Stack{Build: "dockerfile", Name: "Bad_Name", Port: 80}); err == nil {
		t.Error("want validation error for an invalid service name")
	}
}

func TestMCPJSON_Golden(t *testing.T) {
	tests := []struct {
		file string
		o    Options
	}{
		{"mcp_agent_core.json", Options{Names: testNames, APIURL: "https://deploy.example.com"}},
		{"mcp_read_only.json", Options{Names: testNames, Mode: ModeReadOnly}},
		{"mcp_full_explicit.json", Options{Names: testNames, Mode: ModeFull, APIURL: "http://localhost:8080"}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			got, err := MCPJSON(tc.o)
			if err != nil {
				t.Fatalf("MCPJSON: %v", err)
			}
			golden(t, tc.file, got)
			if strings.Contains(got, "Bearer") || !strings.Contains(got, "${"+TokenEnvVar+"}") {
				t.Errorf("token must be an env reference: %s", got)
			}
		})
	}
	if _, err := MCPJSON(Options{Names: testNames, Mode: "bogus"}); err == nil {
		t.Error("want error for unknown mode")
	}
}

func TestAgentsMD_Golden(t *testing.T) {
	tests := []struct {
		file string
		o    Options
	}{
		{"agents_with_url.md", Options{Names: testNames, APIURL: "https://deploy.example.com", App: "site"}},
		{"agents_no_url.md", Options{Names: testNames}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			got, err := AgentsMD(tc.o)
			if err != nil {
				t.Fatalf("AgentsMD: %v", err)
			}
			golden(t, tc.file, got)
			if strings.ContainsRune(got, rune(0x2014)) || strings.ContainsRune(got, rune(0x2013)) {
				t.Error("generated text must not contain em or en dashes")
			}
		})
	}
}
