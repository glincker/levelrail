package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
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

func TestAttemptLogLines_StaysOnTheFailingRelease(t *testing.T) {
	start := time.Now().Add(-time.Minute)
	entries := []telemetry.LogEntry{
		{ContainerID: "old1", Timestamp: start.Add(-time.Hour), Message: "Starting up on port 80"},
		{ContainerID: "old1", Timestamp: start.Add(10 * time.Second), Message: "old still logging"},
		{ContainerID: "new1", Timestamp: start.Add(20 * time.Second), Message: "boom"},
	}
	a := store.DeployAttempt{ServiceName: "web", Image: "app:v2", StartedAt: start}

	// Different images: only the failing release's container counts.
	list := []docker.ContainerState{{ID: "old1", Name: "web-aaaa", Image: "app:v1"}, {ID: "new1", Name: "web-bbbb", Image: "app:v2"}}
	if got := attemptLogLines(entries, list, a); len(got) != 1 || got[0] != "boom" {
		t.Errorf("different images: got %v, want only boom", got)
	}
	// Same image redeployed: the older container predates the attempt.
	list = []docker.ContainerState{
		{ID: "old1", Name: "web-aaaa", Image: "app:v2", Created: start.Add(-time.Hour)},
		{ID: "new1", Name: "web-bbbb", Image: "app:v2", Created: start.Add(time.Second)},
	}
	if got := attemptLogLines(entries, list, a); len(got) != 1 || got[0] != "boom" {
		t.Errorf("shared image: got %v, want only boom", got)
	}
	// The failing container is already gone: older lines still never leak in.
	if got := attemptLogLines(entries, nil, a); len(got) != 2 || got[0] == "Starting up on port 80" {
		t.Errorf("no containers: got %v, want only lines since the attempt started", got)
	}
}
