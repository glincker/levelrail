package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppsDetectJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\nEXPOSE 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := runAppsDetect("levelrail-cli", []string{"--json", dir}, &out, &errb, nil); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var res appDetectResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Detected || res.Build != "dockerfile" || res.Port != 8080 || res.DeployCmd == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}
