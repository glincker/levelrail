package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agentupdate"
)

func noEnv(string) (string, bool) { return "", false }

func TestSelfUpdateNeedsControlPlaneConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSelfUpdate([]string{"--check"}, &out, &errOut, noEnv); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(errOut.String(), envControlPlane) {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestSelfUpdateRejectsConflictingFlags(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSelfUpdate([]string{"--check", "--apply"}, &out, &errOut, noEnv); code != 2 {
		t.Fatalf("code = %d", code)
	}
}

func TestSelfUpdateReadsNoTokenVariable(t *testing.T) {
	var keys []string
	lookup := func(k string) (string, bool) {
		keys = append(keys, k)
		if k == envControlPlane {
			return "127.0.0.1:1", true
		}
		return "", false
	}
	if _, err := buildUpdateOptions(lookup, false); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if strings.Contains(strings.ToUpper(k), "TOKEN") || strings.Contains(k, "API") {
			t.Fatalf("self-update read credential variable %s", k)
		}
	}
}

func TestUsageDocumentsPackageManagedRefusal(t *testing.T) {
	if !strings.Contains(usageText, "package managed") || !strings.Contains(usageText, "exit code 3") {
		t.Fatal("help text must explain the package managed refusal")
	}
	if exitPackageManaged != 3 {
		t.Fatalf("exit code = %d", exitPackageManaged)
	}
}

func TestUpdateStatusLine(t *testing.T) {
	cases := map[string]struct {
		res  agentupdate.Result
		want string
	}{
		"held":      {agentupdate.Result{Action: agentupdate.ActionHold, Current: "v1", Target: "v2", Reason: "window"}, "agent update held: v1 (target v2, reason: window)"},
		"installed": {agentupdate.Result{Action: agentupdate.ActionRestarted, Current: "v1", Target: "v2"}, "agent update installed: v1 -> v2"},
		"checked":   {agentupdate.Result{Action: agentupdate.ActionNone, Current: "v1"}, "agent update checked: up to date at v1"},
	}
	for name, c := range cases {
		if got := updateStatusLine(c.res); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}
