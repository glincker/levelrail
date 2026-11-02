package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPassedStatus(t *testing.T) {
	p := filepath.Join(t.TempDir(), "smoke.json")
	if err := os.WriteFile(p, []byte(`[{"id":"a","status":"pass","http_status":204},{"id":"b","status":"fail"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := passedStatus(p, "a"); err != nil || s != 204 {
		t.Errorf("a = %d, %v", s, err)
	}
	if _, err := passedStatus(p, "b"); err == nil {
		t.Error("failed run must not be recordable")
	}
	if _, err := passedStatus(p, "c"); err == nil {
		t.Error("missing id must error")
	}
}
