package store

import (
	"context"
	"testing"
	"time"
)

func TestRecordAndListProbeAttempts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	started := time.Now().Add(-time.Minute)
	a := DeployAttempt{ID: "dep_probe_1", ServiceName: "web", Image: "nginx:latest", Status: DeployAttemptStatusRunning, StartedAt: started}
	if err := db.SaveDeployAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_probe_1", DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}

	probedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.RecordProbeAttempt(ctx, ProbeAttempt{
		ServiceName: "web",
		Image:       "nginx:latest",
		Target:      "10.0.0.5:8080",
		Success:     false,
		StatusCode:  503,
		LatencyMS:   42,
		ProbedAt:    probedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordProbeAttempt(ctx, ProbeAttempt{
		ServiceName: "web",
		Image:       "nginx:latest",
		Target:      "10.0.0.5:8080",
		Success:     true,
		StatusCode:  200,
		LatencyMS:   7,
		ProbedAt:    probedAt.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	list, err := db.ListProbeAttempts(ctx, "dep_probe_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].Success || list[0].StatusCode != 503 || list[0].Target != "10.0.0.5:8080" || list[0].LatencyMS != 42 {
		t.Errorf("first attempt = %+v", list[0])
	}
	if !list[1].Success || list[1].StatusCode != 200 || list[1].LatencyMS != 7 {
		t.Errorf("second attempt = %+v", list[1])
	}
	if list[0].DeployAttemptID != "dep_probe_1" {
		t.Errorf("DeployAttemptID = %q, want %q", list[0].DeployAttemptID, "dep_probe_1")
	}
}

// TestRecordProbeAttempt_NoMatchingDeployAttempt confirms the best-effort
// contract: a service/image pair with no newest-succeeded deploy_attempts
// row silently inserts nothing rather than erroring, the same tolerance
// RecordRollout already has for a no-match UPDATE.
func TestRecordProbeAttempt_NoMatchingDeployAttempt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.RecordProbeAttempt(ctx, ProbeAttempt{
		ServiceName: "ghost",
		Image:       "nginx:latest",
		Target:      "10.0.0.5:8080",
		Success:     true,
		ProbedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("RecordProbeAttempt() with no matching deploy attempt should be a silent no-op, got error: %v", err)
	}
}
