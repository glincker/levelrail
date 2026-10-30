package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExplainAppPaths(t *testing.T) {
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := runExplain("levelrail-cli", args, &out, &errb, nil)
		return code, out.String(), errb.String()
	}
	code, out, _ := run("replicas")
	if code != exitOK || !strings.Contains(out, "DEFAULT: 1") || !strings.Contains(out, "integer") {
		t.Fatalf("replicas: code %d out %q", code, out)
	}
	code, out, _ = run("services.web.health.readiness")
	if code != exitOK || !strings.Contains(out, "app.yaml") {
		t.Fatalf("nested: code %d out %q", code, out)
	}
	code, out, _ = run("--json", "strategy")
	if code != exitOK || !strings.Contains(out, `"canary"`) || !strings.Contains(out, `"default": "blue-green"`) {
		t.Fatalf("json: code %d out %q", code, out)
	}
	code, out, _ = run("appspec")
	if code != exitOK || !strings.Contains(out, "services") {
		t.Fatalf("appspec: code %d out %q", code, out)
	}
	code, _, errs := run("services.web.replica")
	if code == exitOK || !strings.Contains(errs, "did you mean") {
		t.Fatalf("unknown path: code %d err %q", code, errs)
	}
}
