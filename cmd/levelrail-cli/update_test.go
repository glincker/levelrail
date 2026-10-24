package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/version"
)

// updateTestEnv serves a one-release listing and points the CLI at it.
func updateTestEnv(t *testing.T, tag string, extra map[string]string) (env func(string) (string, bool), hits *atomic.Int64) {
	t.Helper()
	old := version.Version
	version.Version = "v0.0.1"
	t.Cleanup(func() { version.Version = old })
	hits = &atomic.Int64{}
	asset := fmt.Sprintf("levelrail-cli-%s-%s", runtime.GOOS, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = fmt.Fprintf(w, `[{"tag_name":%q,"prerelease":false,"assets":[{"name":%q}]}]`, tag, asset)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	vars := map[string]string{envReleaseAPIURL: srv.URL, envInsecureMirror: "1"}
	for k, v := range extra {
		vars[k] = v
	}
	return func(k string) (string, bool) { v, ok := vars[k]; return v, ok }, hits
}

func TestUpdateCheckOutdatedExitsZero(t *testing.T) {
	env, _ := updateTestEnv(t, "v0.2.0", nil)
	var out, errb bytes.Buffer
	if code := runUpdate("levelrail-cli", []string{"--check"}, &out, &errb, env); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	for _, want := range []string{"v0.0.1", "v0.2.0", "update is available"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %s", want, out.String())
		}
	}
}

func TestUpdateCheckJSON(t *testing.T) {
	env, _ := updateTestEnv(t, "v0.2.0", nil)
	var out, errb bytes.Buffer
	if code := runUpdate("levelrail-cli", []string{"--check", "--json"}, &out, &errb, env); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	var got updateCheckReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not one JSON value: %v: %s", err, out.String())
	}
	if got.Current != "v0.0.1" || got.Latest != "v0.2.0" || !got.UpdateAvailable {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateCheckUpToDate(t *testing.T) {
	env, _ := updateTestEnv(t, "v0.0.1", nil)
	var out, errb bytes.Buffer
	if code := runUpdate("levelrail-cli", []string{"--check", "--output", "json"}, &out, &errb, env); code != exitOK {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), `"update_available": false`) {
		t.Fatalf("out=%s", out.String())
	}
}

func TestUpdateRequiresYesInCIMode(t *testing.T) {
	env, _ := updateTestEnv(t, "v0.2.0", nil)
	old := ciMode
	ciMode = true
	t.Cleanup(func() { ciMode = old })
	exe := t.TempDir() + "/levelrail-cli"
	oldExe := updateExePath
	updateExePath = func() (string, error) { return exe, nil }
	t.Cleanup(func() { updateExePath = oldExe })
	var out, errb bytes.Buffer
	code := runUpdate("levelrail-cli", nil, &out, &errb, env)
	if code != exitValidation || !strings.Contains(errb.String(), "--yes") {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
}

func TestUpdateDryRunPrintsPlan(t *testing.T) {
	env, _ := updateTestEnv(t, "v0.2.0", nil)
	exe := t.TempDir() + "/levelrail-cli"
	oldExe := updateExePath
	updateExePath = func() (string, error) { return exe, nil }
	t.Cleanup(func() { updateExePath = oldExe })
	var out, errb bytes.Buffer
	if code := runUpdate("levelrail-cli", []string{"--dry-run", "--yes"}, &out, &errb, env); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "would replace") || !strings.Contains(out.String(), "Nothing was changed") {
		t.Fatalf("out=%s", out.String())
	}
}

func TestUpdateHintMakesNoRequestInCIOrJSONOrOptOut(t *testing.T) {
	cases := map[string]struct {
		args  []string
		extra map[string]string
		force bool
	}{
		"CI env":      {[]string{"apps", "list"}, map[string]string{"CI": "true"}, false},
		"no terminal": {[]string{"apps", "list"}, nil, true},
		"json":        {[]string{"apps", "list", "--json"}, nil, false},
		"output json": {[]string{"apps", "list", "--output", "json"}, nil, false},
		"opt out":     {[]string{"apps", "list"}, map[string]string{envNoUpdateCheck: "1"}, false},
		"interval 0":  {[]string{"apps", "list"}, map[string]string{envUpdateCheckDays: "0"}, false},
		"update verb": {[]string{"update", "--check"}, nil, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env, hits := updateTestEnv(t, "v0.2.0", c.extra)
			old := forceCI
			forceCI = c.force
			t.Cleanup(func() { forceCI = old })
			var errb bytes.Buffer
			startUpdateHint("levelrail-cli", c.args, &errb, env)()
			if hits.Load() != 0 || errb.Len() != 0 {
				t.Fatalf("hits=%d stderr=%q", hits.Load(), errb.String())
			}
		})
	}
}

func TestUpdateHintPrintsOneLine(t *testing.T) {
	env, _ := updateTestEnv(t, "v0.2.0", map[string]string{envUpdateHintWaitMS: "5000"})
	old := forceCI
	forceCI = false
	t.Cleanup(func() { forceCI = old })
	var errb bytes.Buffer
	startUpdateHint("levelrail-cli", []string{"apps", "list"}, &errb, env)()
	if got := strings.TrimSpace(errb.String()); strings.Count(got, "\n") != 0 || !strings.Contains(got, "v0.2.0") {
		t.Fatalf("stderr=%q", got)
	}
}

func TestUpdateCheckExitCodeFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		tag  string
		args []string
		want int
	}{
		{"outdated default stays 0", "v0.2.0", []string{"--check"}, exitOK},
		{"outdated with flag is 10", "v0.2.0", []string{"--check", "--exit-code"}, exitUpdateAvail},
		{"current with flag is 0", "v0.0.1", []string{"--check", "--exit-code"}, exitOK},
		{"self-update style version shortcut", "v0.2.0", []string{"--check", "--exit-code", "--channel", "beta"}, exitUpdateAvail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := updateTestEnv(t, tc.tag, nil)
			var out, errb bytes.Buffer
			if code := runUpdate("levelrail-cli", tc.args, &out, &errb, env); code != tc.want {
				t.Fatalf("code=%d want %d stderr=%s", code, tc.want, errb.String())
			}
		})
	}
}

func TestVersionCheckAndSelfUpdateAlias(t *testing.T) {
	env, _ := updateTestEnv(t, "v0.2.0", nil)
	for _, args := range [][]string{{"version", "--check", "--exit-code"}, {"self-update", "--check", "--exit-code"}} {
		var out, errb bytes.Buffer
		if code := run("levelrail-cli", args, &out, &errb, env); code != exitUpdateAvail {
			t.Fatalf("%v: code=%d stderr=%s", args, code, errb.String())
		}
	}
}
