package failure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func TestBuildDiskPreflightIsDiskFull(t *testing.T) {
	in := Input{Status: "failed", Error: `deploy: service "web": build: build: insufficient disk space at "/tmp/build-1": 957280256 bytes free, need at least 1073741824 bytes (set APP_MIN_BUILD_DISK_MB to override)`}
	got, ok := Classify(in, Options{})
	if !ok || got.Code != CodeDiskFull {
		t.Fatalf("Classify() = %+v, %v, want %s", got, ok, CodeDiskFull)
	}
}

func TestDockerfileMissingSuggestsRailpack(t *testing.T) {
	in := Input{Status: "failed", Error: `deploy: service "web": build: failed to read dockerfile: open Dockerfile: no such file or directory`}
	got, ok := Classify(in, Options{})
	if !ok || got.Code != CodeDockerfileError {
		t.Fatalf("Classify() = %+v, %v, want %s", got, ok, CodeDockerfileError)
	}
	if !strings.Contains(got.SuggestedFix, "--build-type railpack") {
		t.Errorf("SuggestedFix = %q, want it to name --build-type railpack", got.SuggestedFix)
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

func TestEdgeCases(t *testing.T) {
	ansi := "\x1b[31mnpm ERR!\x1b[0m \x1b[1mno space left on\x1b[0m device"
	cases := []struct {
		name string
		in   Input
		code string
		also []string
	}{
		{"empty log is unknown", Input{Status: "failed"}, CodeUnknown, nil},
		{"ansi codes do not hide a signature", Input{Status: "failed", LogLines: []string{"\x1b[31mexec format error\x1b[0m"}}, CodePlatformMismatch, nil},
		{"ansi split inside a phrase is stripped", Input{Status: "failed", LogLines: []string{ansi}}, CodeDiskFull, nil},
		{"most specific class wins and others are listed", Input{Status: "failed", FailingStep: "building", Error: "build: x", LogLines: []string{"npm error code EUSAGE", "npm ci can only install packages when your package.json and package-lock.json are in sync"}}, CodeLockfileOutOfSync, []string{CodeDependencyInstall}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Classify(tc.in, Options{})
			if !ok || got.Code != tc.code {
				t.Fatalf("got %+v ok=%v, want code %s", got, ok, tc.code)
			}
			if strings.Join(got.AlsoMatched, ",") != strings.Join(tc.also, ",") {
				t.Errorf("also_matched = %v, want %v", got.AlsoMatched, tc.also)
			}
		})
	}
}

func TestVeryLongLogStillClassifies(t *testing.T) {
	logs := make([]string, 0, 200000)
	for i := 0; i < 199990; i++ {
		logs = append(logs, "#5 compiling module "+strconv.Itoa(i))
	}
	logs = append(logs, "npm ci can only install packages when your package.json and package-lock.json are in sync")
	start := time.Now()
	got, _ := Classify(Input{Status: "failed", FailingStep: "building", Error: "build: x", LogLines: logs}, Options{})
	if got.Code != CodeLockfileOutOfSync {
		t.Fatalf("code = %s", got.Code)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("classification took %s", d)
	}
}

func TestBuildStepVersusDependencyInstall(t *testing.T) {
	at := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		logs []string
		want string
	}{
		{"plain RUN exit code", []string{`process "/bin/sh -c echo x && exit 7" did not complete successfully: exit code: 7`}, CodeBuildStepFailed},
		{"npm error", []string{`npm ERR! code E404`, `process "/bin/sh -c npm install" did not complete successfully: exit code: 1`}, CodeDependencyInstall},
		{"pip error", []string{`ERROR: No matching distribution found for nope`, `process "/bin/sh -c pip install nope" did not complete successfully: exit code: 1`}, CodeDependencyInstall},
		{"apk error", []string{`ERROR: unable to select packages:`, `process "/bin/sh -c apk add nope" did not complete successfully: exit code: 1`}, CodeDependencyInstall},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Classify(Input{Status: "failed", FailingStep: "building", Error: "build: build: exit 1", LogLines: tc.logs, At: at}, Options{DocsBase: "https://docs.example.test"})
			if !ok || got.Code != tc.want {
				t.Fatalf("code = %q, want %q", got.Code, tc.want)
			}
			if strings.Contains(got.LogExcerpt, "build: build:") || !strings.HasPrefix(got.DocsURL, "https://docs.example.test/") {
				t.Errorf("excerpt or docs url not polished: %q %q", got.LogExcerpt, got.DocsURL)
			}
		})
	}
}
