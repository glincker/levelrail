package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The end-to-end run builds and boots a real control plane, so it is opt-in.
func TestSmallProfileEndToEnd(t *testing.T) {
	if testing.Short() || os.Getenv("LEVELRAIL_STRESS_E2E") != "1" {
		t.Skip("set LEVELRAIL_STRESS_E2E=1 to run the end-to-end stress test")
	}
	report := filepath.Join(t.TempDir(), "report.json")
	var out, errb bytes.Buffer
	code := realMain([]string{"-profile", "../../stress/profiles/small.yaml", "-repo", "../..", "-duration", "20s", "-report", report}, &out, &errb)
	t.Log(out.String())
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if _, err := os.Stat(report); err != nil {
		t.Fatal(err)
	}
}
