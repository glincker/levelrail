package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewDeployAttemptID(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		id, err := NewDeployAttemptID()
		if err != nil {
			t.Fatalf("NewDeployAttemptID() error = %v", err)
		}
		if id == "" {
			t.Fatal("NewDeployAttemptID() returned empty string")
		}
		if len(id) < len(deployAttemptIDPrefix)+10 {
			t.Errorf("NewDeployAttemptID() = %q, looks too short to be random bytes plus prefix", id)
		}
		if id[:len(deployAttemptIDPrefix)] != deployAttemptIDPrefix {
			t.Errorf("NewDeployAttemptID() = %q, want prefix %q", id, deployAttemptIDPrefix)
		}
		if seen[id] {
			t.Fatalf("NewDeployAttemptID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestSaveAndGetDeployAttempt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	started := time.Now().UTC().Truncate(time.Millisecond)
	want := DeployAttempt{
		ID:          "dep_test1",
		ServiceName: "web",
		Image:       "levelrail/web:abc123",
		Status:      DeployAttemptStatusRunning,
		StartedAt:   started,
	}
	if err := db.SaveDeployAttempt(ctx, want); err != nil {
		t.Fatalf("SaveDeployAttempt() error = %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_test1")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.ID != want.ID || got.ServiceName != want.ServiceName || got.Image != want.Image || got.Status != want.Status {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}
	if got.FinishedAt != nil {
		t.Errorf("FinishedAt = %v, want nil before the attempt finishes", got.FinishedAt)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty before the attempt finishes", got.Error)
	}
}

func TestGetDeployAttempt_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetDeployAttempt(context.Background(), "dep_never-saved")
	if !errors.Is(err, ErrDeployAttemptNotFound) {
		t.Errorf("error = %v, want ErrDeployAttemptNotFound", err)
	}
}

func TestFinishDeployAttempt_Succeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_ok", ServiceName: "web", Image: "levelrail/web:abc123",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	finished := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.FinishDeployAttempt(ctx, "dep_ok", DeployAttemptStatusSucceeded, finished, ""); err != nil {
		t.Fatalf("FinishDeployAttempt() error = %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_ok")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.Status != DeployAttemptStatusSucceeded {
		t.Errorf("Status = %q, want %q", got.Status, DeployAttemptStatusSucceeded)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finished) {
		t.Errorf("FinishedAt = %v, want %v", got.FinishedAt, finished)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty for a succeeded attempt", got.Error)
	}
}

func TestFinishDeployAttempt_Failed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_fail", ServiceName: "web", Image: "levelrail/web:abc123",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	finished := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.FinishDeployAttempt(ctx, "dep_fail", DeployAttemptStatusFailed, finished, "build: step failed"); err != nil {
		t.Fatalf("FinishDeployAttempt() error = %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_fail")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.Status != DeployAttemptStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, DeployAttemptStatusFailed)
	}
	if got.Error != "build: step failed" {
		t.Errorf("Error = %q, want %q", got.Error, "build: step failed")
	}
}

func TestFinishDeployAttempt_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.FinishDeployAttempt(context.Background(), "dep_ghost", DeployAttemptStatusFailed, time.Now(), "boom")
	if !errors.Is(err, ErrDeployAttemptNotFound) {
		t.Errorf("error = %v, want ErrDeployAttemptNotFound", err)
	}
}

func TestListDeployAttempts_NewestFirstScopedToService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Millisecond)
	attempts := []DeployAttempt{
		{ID: "dep_1", ServiceName: "web", Image: "web:1", Status: DeployAttemptStatusSucceeded, StartedAt: base},
		{ID: "dep_2", ServiceName: "web", Image: "web:2", Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Minute)},
		{ID: "dep_3", ServiceName: "worker", Image: "worker:1", Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(2 * time.Minute)},
	}
	for _, a := range attempts {
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed %s: %v", a.ID, err)
		}
	}

	got, err := db.ListDeployAttempts(ctx, "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 attempts for service %q, got %d", "web", len(got))
	}
	if got[0].ID != "dep_2" || got[1].ID != "dep_1" {
		t.Errorf("got IDs [%s, %s], want newest first [dep_2, dep_1]", got[0].ID, got[1].ID)
	}
}

func TestListDeployAttempts_EmptyIsNotError(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListDeployAttempts(context.Background(), "never-deployed")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no attempts, got %d", len(got))
	}
}
