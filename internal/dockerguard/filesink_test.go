package dockerguard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileSinkWritesPrivateLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileSinkName)
	sink := NewFileSink(path)
	d := Decision{Denied: true, Mode: ModeEnforce, Method: "POST", Path: "/containers/create", Container: "x", At: time.Now(),
		Violations: []Violation{{Rule: RulePrivileged, Reason: "HostConfig.Privileged is never allowed"}}}
	for i := 0; i < 2; i++ {
		if err := sink.RecordDecision(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(b), "\n"); lines != 2 || !strings.Contains(string(b), `"action":"docker_guard.denied"`) {
		t.Fatalf("audit file = %s", b)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("audit file mode: %v %v", info, err)
	}
}
