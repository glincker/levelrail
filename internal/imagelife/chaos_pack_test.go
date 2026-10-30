package imagelife

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// After a deploy fails on a corrupt image, the serving release and its
// rollback pin must survive even the most aggressive cleanup policy.
func TestChaosPack_FailedDeploy_ServingReleaseAndPinSurviveAggressiveGC(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	records := []docker.ImageRecord{
		{ID: "sha256:v1", Tags: []string{"app:v1"}, SizeBytes: 100, CreatedAt: now.Add(-30 * day)},
		{ID: "sha256:v1", Tags: []string{"pin/app:d1"}, SizeBytes: 100, CreatedAt: now.Add(-30 * day)},
		{ID: "sha256:v2corrupt", Tags: []string{"app:v2"}, SizeBytes: 200, CreatedAt: now.Add(-day)},
	}
	app := AppState{
		Name:      "web",
		LiveImage: "app:v1",
		Pins:      []Pin{{DeployID: "d1", ImageID: "sha256:v1", PinRef: "pin/app:d1"}},
		Attempts: []Attempt{
			{ID: "d2", Image: "app:v2", Status: "failed"},
			{ID: "d1", Image: "app:v1", Status: "succeeded"},
		},
	}

	plan := PlanGC(Build(records, []AppState{app}, 0, 0, now))
	for _, e := range plan.Remove {
		if e.ID == "sha256:v1" {
			t.Fatalf("GC plan removes the serving release or its pin: %+v", e)
		}
	}
}
