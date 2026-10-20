package store

import (
	"context"
	"errors"
	"testing"
)

func newTestNetworkShare() NetworkShare {
	return NetworkShare{
		ID:         "ns_test1",
		Name:       "media-nas",
		Protocol:   NetworkShareProtocolNFS,
		Host:       "nas.lan",
		RemotePath: "/export/media",
		CreatedAt:  "2026-10-01T00:00:00Z",
	}
}

func TestSaveAndGetNetworkShare(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestNetworkShare()
	if err := db.SaveNetworkShare(ctx, want); err != nil {
		t.Fatalf("SaveNetworkShare() error = %v", err)
	}

	got, err := db.GetNetworkShare(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetNetworkShare() error = %v", err)
	}
	if got != want {
		t.Errorf("GetNetworkShare() = %+v, want %+v", got, want)
	}
}

func TestGetNetworkShare_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetNetworkShare(ctx, "ns_missing")
	if !errors.Is(err, ErrNetworkShareNotFound) {
		t.Fatalf("GetNetworkShare() error = %v, want ErrNetworkShareNotFound", err)
	}
}

func TestGetNetworkShareByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestNetworkShare()
	if err := db.SaveNetworkShare(ctx, want); err != nil {
		t.Fatalf("SaveNetworkShare() error = %v", err)
	}

	got, err := db.GetNetworkShareByName(ctx, want.Name)
	if err != nil {
		t.Fatalf("GetNetworkShareByName() error = %v", err)
	}
	if got != want {
		t.Errorf("GetNetworkShareByName() = %+v, want %+v", got, want)
	}
}

func TestGetNetworkShareByName_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetNetworkShareByName(ctx, "does-not-exist")
	if !errors.Is(err, ErrNetworkShareNotFound) {
		t.Fatalf("GetNetworkShareByName() error = %v, want ErrNetworkShareNotFound", err)
	}
}

func TestListNetworkShares_OrderedByCreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestNetworkShare()
	first.ID, first.Name, first.CreatedAt = "ns_a", "share-a", "2026-10-01T00:00:00Z"
	second := newTestNetworkShare()
	second.ID, second.Name, second.CreatedAt = "ns_b", "share-b", "2026-10-01T00:00:01Z"

	if err := db.SaveNetworkShare(ctx, second); err != nil {
		t.Fatalf("SaveNetworkShare(second) error = %v", err)
	}
	if err := db.SaveNetworkShare(ctx, first); err != nil {
		t.Fatalf("SaveNetworkShare(first) error = %v", err)
	}

	got, err := db.ListNetworkShares(ctx)
	if err != nil {
		t.Fatalf("ListNetworkShares() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "ns_a" || got[1].ID != "ns_b" {
		t.Fatalf("ListNetworkShares() = %+v, want [ns_a, ns_b] in creation order", got)
	}
}

func TestUpdateNetworkShare(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	share := newTestNetworkShare()
	if err := db.SaveNetworkShare(ctx, share); err != nil {
		t.Fatalf("SaveNetworkShare() error = %v", err)
	}

	err := db.UpdateNetworkShare(ctx, share.ID, "renamed", NetworkShareProtocolCIFS, "nas2.lan", "/backups", "vers=3.0", "backup-user")
	if err != nil {
		t.Fatalf("UpdateNetworkShare() error = %v", err)
	}

	got, err := db.GetNetworkShare(ctx, share.ID)
	if err != nil {
		t.Fatalf("GetNetworkShare() error = %v", err)
	}
	if got.Name != "renamed" || got.Protocol != NetworkShareProtocolCIFS || got.Host != "nas2.lan" ||
		got.RemotePath != "/backups" || got.MountOptions != "vers=3.0" || got.Username != "backup-user" {
		t.Errorf("GetNetworkShare() after update = %+v, want renamed/cifs/nas2.lan/backups/vers=3.0/backup-user", got)
	}
}

func TestUpdateNetworkShare_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateNetworkShare(ctx, "ns_missing", "x", NetworkShareProtocolNFS, "host", "/path", "", "")
	if !errors.Is(err, ErrNetworkShareNotFound) {
		t.Fatalf("UpdateNetworkShare() error = %v, want ErrNetworkShareNotFound", err)
	}
}

func TestDeleteNetworkShare(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestNetworkShare()
	if err := db.SaveNetworkShare(ctx, want); err != nil {
		t.Fatalf("SaveNetworkShare() error = %v", err)
	}
	if err := db.DeleteNetworkShare(ctx, want.ID); err != nil {
		t.Fatalf("DeleteNetworkShare() error = %v", err)
	}

	_, err := db.GetNetworkShare(ctx, want.ID)
	if !errors.Is(err, ErrNetworkShareNotFound) {
		t.Fatalf("GetNetworkShare() after delete error = %v, want ErrNetworkShareNotFound", err)
	}
}

func TestDeleteNetworkShare_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteNetworkShare(ctx, "ns_missing")
	if !errors.Is(err, ErrNetworkShareNotFound) {
		t.Fatalf("DeleteNetworkShare() error = %v, want ErrNetworkShareNotFound", err)
	}
}

func TestSaveNetworkShare_DuplicateName_Rejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestNetworkShare()
	if err := db.SaveNetworkShare(ctx, first); err != nil {
		t.Fatalf("SaveNetworkShare(first) error = %v", err)
	}

	second := newTestNetworkShare()
	second.ID = "ns_other"
	if err := db.SaveNetworkShare(ctx, second); err == nil {
		t.Fatal("SaveNetworkShare() error = nil, want a UNIQUE constraint error for a duplicate name")
	}
}
