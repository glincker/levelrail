package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNodeProvision_SaveGetList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	p := NodeProvision{
		ID: "npv_1", Provider: "hetzner", Region: "fsn1", Size: "cx22",
		Name: "web-1", Role: "general", Status: NodeProvisionStatusCreating,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveNodeProvision(ctx, p); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	got, err := db.GetNodeProvision(ctx, "npv_1")
	if err != nil {
		t.Fatalf("GetNodeProvision: %v", err)
	}
	if got.Name != "web-1" || got.Provider != "hetzner" || got.Status != NodeProvisionStatusCreating {
		t.Errorf("got = %+v", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}

	list, err := db.ListNodeProvisions(ctx)
	if err != nil {
		t.Fatalf("ListNodeProvisions: %v", err)
	}
	if len(list) != 1 || list[0].ID != "npv_1" {
		t.Errorf("list = %+v", list)
	}
}

func TestNodeProvision_GetNotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetNodeProvision(context.Background(), "missing")
	if !errors.Is(err, ErrNodeProvisionNotFound) {
		t.Errorf("err = %v, want ErrNodeProvisionNotFound", err)
	}
}

func TestNodeProvision_UpdateStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	p := NodeProvision{
		ID: "npv_2", Provider: "digitalocean", Region: "nyc1", Size: "s-1vcpu-1gb",
		Name: "build-1", Role: "build", Status: NodeProvisionStatusCreating,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveNodeProvision(ctx, p); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	later := now.Add(time.Minute)
	if err := db.UpdateNodeProvisionStatus(ctx, "npv_2", NodeProvisionStatusReady, "srv_999", "1.2.3.4", "node_abc", "", later); err != nil {
		t.Fatalf("UpdateNodeProvisionStatus: %v", err)
	}

	got, err := db.GetNodeProvision(ctx, "npv_2")
	if err != nil {
		t.Fatalf("GetNodeProvision: %v", err)
	}
	if got.Status != NodeProvisionStatusReady || got.IPAddress != "1.2.3.4" || got.NodeID != "node_abc" || got.ProviderServerID != "srv_999" {
		t.Errorf("got = %+v", got)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
	}

	// A second update with an empty providerServerID must leave the
	// already-stored one alone: most callers (a live status refresh) only
	// ever learn status/ip/node/failure after the server already has one.
	if err := db.UpdateNodeProvisionStatus(ctx, "npv_2", NodeProvisionStatusReady, "", "1.2.3.5", "node_abc", "", later.Add(time.Minute)); err != nil {
		t.Fatalf("UpdateNodeProvisionStatus: %v", err)
	}
	got, err = db.GetNodeProvision(ctx, "npv_2")
	if err != nil {
		t.Fatalf("GetNodeProvision: %v", err)
	}
	if got.ProviderServerID != "srv_999" {
		t.Errorf("ProviderServerID = %q, want srv_999 (unchanged)", got.ProviderServerID)
	}
}

func TestNodeProvision_UpdateStatus_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateNodeProvisionStatus(context.Background(), "missing", NodeProvisionStatusFailed, "", "", "", "boom", time.Now())
	if !errors.Is(err, ErrNodeProvisionNotFound) {
		t.Errorf("err = %v, want ErrNodeProvisionNotFound", err)
	}
}

func TestNodeProviderSecretsKey(t *testing.T) {
	if got, want := NodeProviderSecretsKey("hetzner"), "node-provider/hetzner"; got != want {
		t.Errorf("NodeProviderSecretsKey(hetzner) = %q, want %q", got, want)
	}
}
