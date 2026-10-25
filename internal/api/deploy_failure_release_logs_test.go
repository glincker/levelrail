package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestFilterLinesByContainer(t *testing.T) {
	entries := []telemetry.LogEntry{
		{ContainerID: "oldold", Message: "listening on 8080"},
		{ContainerID: "newnew123", Message: "dial tcp refused"},
	}
	got := filterLinesByContainer(entries, []string{"newnew123456"})
	if len(got) != 1 || got[0] != "dial tcp refused" {
		t.Errorf("got %v, want only the failing release's line", got)
	}
	if all := filterLinesByContainer(entries, nil); len(all) != 2 {
		t.Errorf("no known containers must keep every line, got %v", all)
	}
}
