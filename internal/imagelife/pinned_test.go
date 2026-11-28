package imagelife

import (
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/docker"
)

func TestPinnedDeployImageIsProtected(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	records := []docker.ImageRecord{
		rec("sha256:live", 100, 10*day, now, "app:v3"),
		rec("sha256:keepme", 200, 30*day, now, "app:v1"),
		rec("sha256:gone", 300, 30*day, now, "app:v0"),
	}
	build := func(pinned bool) []Entry {
		app := AppState{Name: "web", LiveImage: "app:v3", Keep: 1, Attempts: []Attempt{
			{ID: "d3", Image: "app:v3", Status: "succeeded"},
			{ID: "d1", Image: "app:v1", Status: "succeeded", Pinned: pinned},
			{ID: "d0", Image: "app:v0", Status: "succeeded"},
		}}
		return Build(records, []AppState{app}, 1, 0, now)
	}
	removed := func(entries []Entry) map[string]bool {
		out := map[string]bool{}
		for _, e := range PlanGC(entries).Remove {
			out[e.ID] = true
		}
		return out
	}
	if r := removed(build(true)); r["sha256:keepme"] || !r["sha256:gone"] {
		t.Fatalf("pinned pass removes %v, want only sha256:gone", r)
	}
	if r := removed(build(false)); !r["sha256:keepme"] {
		t.Fatalf("unpinned pass removes %v, want sha256:keepme released", r)
	}
	for _, e := range build(true) {
		if e.ID == "sha256:keepme" && e.Removable {
			t.Error("an explicit removal of a pinned deploy image must be refused")
		}
	}
}
