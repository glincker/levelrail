package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSSHNodeProvision_SaveGetList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	p := SSHNodeProvision{
		ID: "sshp_1", Name: "web-1", Role: "general",
		Status: SSHNodeProvisionStatusConnecting, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveSSHNodeProvision(ctx, p); err != nil {
		t.Fatalf("SaveSSHNodeProvision: %v", err)
	}

	got, err := db.GetSSHNodeProvision(ctx, "sshp_1")
	if err != nil {
		t.Fatalf("GetSSHNodeProvision: %v", err)
	}
	if got.Name != "web-1" || got.Status != SSHNodeProvisionStatusConnecting {
		t.Errorf("got = %+v", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}

	list, err := db.ListSSHNodeProvisions(ctx)
	if err != nil {
		t.Fatalf("ListSSHNodeProvisions: %v", err)
	}
	if len(list) != 1 || list[0].ID != "sshp_1" {
		t.Errorf("list = %+v", list)
	}
}

func TestSSHNodeProvision_GetNotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetSSHNodeProvision(context.Background(), "missing")
	if !errors.Is(err, ErrSSHNodeProvisionNotFound) {
		t.Errorf("err = %v, want ErrSSHNodeProvisionNotFound", err)
	}
}

func TestSSHNodeProvision_UpdateProgress(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	p := SSHNodeProvision{
		ID: "sshp_2", Name: "build-1", Role: "build",
		Status: SSHNodeProvisionStatusConnecting, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveSSHNodeProvision(ctx, p); err != nil {
		t.Fatalf("SaveSSHNodeProvision: %v", err)
	}

	later := now.Add(time.Minute)
	if err := db.UpdateSSHNodeProvisionProgress(ctx, "sshp_2", SSHNodeProvisionStatusReady,
		"linux", "amd64", "connecting...\ndetecting...\nready", "node_abc", "", later); err != nil {
		t.Fatalf("UpdateSSHNodeProvisionProgress: %v", err)
	}

	got, err := db.GetSSHNodeProvision(ctx, "sshp_2")
	if err != nil {
		t.Fatalf("GetSSHNodeProvision: %v", err)
	}
	if got.Status != SSHNodeProvisionStatusReady || got.DetectedOS != "linux" || got.DetectedArch != "amd64" || got.NodeID != "node_abc" {
		t.Errorf("got = %+v", got)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
	}
}

func TestSSHNodeProvision_UpdateProgressNotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateSSHNodeProvisionProgress(context.Background(), "missing", SSHNodeProvisionStatusFailed, "", "", "", "", "boom", time.Now())
	if !errors.Is(err, ErrSSHNodeProvisionNotFound) {
		t.Errorf("err = %v, want ErrSSHNodeProvisionNotFound", err)
	}
}
