package failure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClassifyFixtures runs every testdata/failure/<class>/ directory: input.json
// is an Input, want.json the expected code and retryable flag.
func TestClassifyFixtures(t *testing.T) {
	at := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	dirs, err := os.ReadDir("testdata/failure")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		dir := filepath.Join("testdata/failure", d.Name())
		var want struct {
			Code      string   `json:"code"`
			Retryable bool     `json:"retryable"`
			Excerpt   []string `json:"excerpt_contains"`
		}
		readJSON(t, filepath.Join(dir, "want.json"), &want)
		seen[want.Code] = true
		t.Run(d.Name(), func(t *testing.T) {
			var in Input
			readJSON(t, filepath.Join(dir, "input.json"), &in)
			in.App, in.DeployID, in.At = "web", "da_1", at
			got, ok := Classify(in, Options{})
			if !ok {
				t.Fatalf("Classify reported not failing")
			}
			if got.Code != want.Code {
				t.Fatalf("code = %q, want %q (cause %q)", got.Code, want.Code, got.Cause)
			}
			if got.Retryable != want.Retryable {
				t.Errorf("retryable = %v, want %v", got.Retryable, want.Retryable)
			}
			for _, frag := range want.Excerpt {
				if !strings.Contains(got.LogExcerpt, frag) {
					t.Errorf("excerpt missing decisive line %q:\n%s", frag, got.LogExcerpt)
				}
			}
			if got.SuggestedFix == "" || got.Cause == "" || got.DocsURL != "/deploy-failures#"+want.Code {
				t.Errorf("incomplete object: %+v", got)
			}
			if got.App != "web" || got.DeployID != "da_1" || !got.At.Equal(at) {
				t.Errorf("identity fields not carried: %+v", got)
			}
		})
	}
	for _, c := range classes {
		if !seen[c.code] {
			t.Errorf("class %q has no fixture under testdata/failure", c.code)
		}
	}
	if !seen[CodeUnknown] {
		t.Error("no fixture for unknown")
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // fixture paths come from the repo's own testdata directory
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestClassifyNotFailing(t *testing.T) {
	for _, status := range []string{"succeeded", "running", "queued", "canceled", "superseded"} {
		if _, ok := Classify(Input{Status: status}, Options{}); ok {
			t.Errorf("status %q classified as failing", status)
		}
	}
	if _, ok := Classify(Input{Status: "succeeded", Condition: &Condition{Reason: "Deployed"}}, Options{}); ok {
		t.Error("healthy rollout classified as failing")
	}
}

func TestUnknownKeepsExcerpt(t *testing.T) {
	got, _ := Classify(Input{Status: "failed", Error: "weird failure", LogLines: []string{"line a", "line b"}}, Options{})
	if got.Code != CodeUnknown || !strings.Contains(got.LogExcerpt, "weird failure") || !strings.Contains(got.LogExcerpt, "line b") {
		t.Errorf("unknown failure lost its excerpt: %+v", got)
	}
}

func TestExcerptCappedAndRedacted(t *testing.T) {
	var logs []string
	for i := 0; i < 100; i++ {
		logs = append(logs, fmt.Sprintf("step %d output", i))
	}
	logs = append(logs, "npm ERR! code ERESOLVE", "curl https://user:hunter2@registry.example.com/x", "Authorization: Bearer abcdefghijklmnop12345", "trailing 1", "trailing 2", "trailing 3")
	got, _ := Classify(Input{Status: "failed", Error: "build: x", LogLines: logs}, Options{MaxLines: 5, MaxBytes: 4000})
	lines := strings.Split(got.LogExcerpt, "\n")
	if len(lines) > 5 {
		t.Errorf("excerpt has %d lines, want at most 5", len(lines))
	}
	if strings.Contains(got.LogExcerpt, "hunter2") || strings.Contains(got.LogExcerpt, "abcdefghijklmnop12345") {
		t.Errorf("excerpt leaked a secret: %q", got.LogExcerpt)
	}
	if strings.Contains(got.LogExcerpt, "step 3 output") {
		t.Errorf("excerpt is not the tail: %q", got.LogExcerpt)
	}

	capped, _ := Classify(Input{Status: "failed", Error: "weird", LogLines: []string{strings.Repeat("x", 500)}}, Options{MaxBytes: 50})
	if len(capped.LogExcerpt) > 50+len("...[truncated 999 bytes]") {
		t.Errorf("excerpt not byte capped: %d", len(capped.LogExcerpt))
	}
}

func TestOptionsFromEnv(t *testing.T) {
	env := map[string]string{EnvExcerptLines: "7", EnvExcerptBytes: "bad"}
	got := OptionsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if got.MaxLines != 7 || got.MaxBytes != defaultExcerptBytes {
		t.Errorf("options = %+v", got)
	}
}
