package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDoctorCheckMountHelper(t *testing.T) {
	found := func(string) (string, error) { return "/usr/sbin/mount.nfs", nil }
	notFound := func(string) (string, error) { return "", errors.New("not found") }

	ok := doctorCheckMountHelper(found, "mount.nfs", "NFS client tools", "nfs-common")
	if ok.Status != doctorStatusOK {
		t.Errorf("Status = %q, want %q when the binary is found", ok.Status, doctorStatusOK)
	}

	missing := doctorCheckMountHelper(notFound, "mount.nfs", "NFS client tools", "nfs-common")
	if missing.Status != doctorStatusFail {
		t.Errorf("Status = %q, want %q when the binary is missing", missing.Status, doctorStatusFail)
	}
	if missing.Fix == "" {
		t.Error("Fix is empty, want an install hint when the binary is missing")
	}
}

func TestDoctorCheckNASClientTools_NoSharesConfigured(t *testing.T) {
	rt, db := newTestRouterWithNetworkShareSecrets(t, &fakeNetworkShareSecretsSetter{})
	_ = db

	got := rt.doctorCheckNASClientTools(context.Background())
	if len(got) != 0 {
		t.Errorf("doctorCheckNASClientTools() = %+v, want no checks when no network share is configured", got)
	}
}

func TestDoctorCheckNASClientTools_OnlyChecksConfiguredProtocols(t *testing.T) {
	rt, db := newTestRouterWithNetworkShareSecrets(t, &fakeNetworkShareSecretsSetter{})
	ctx := context.Background()

	if err := db.SaveNetworkShare(ctx, store.NetworkShare{
		ID: "ns_1", Name: "media-nas", Protocol: store.NetworkShareProtocolNFS,
		Host: "nas.lan", RemotePath: "/export/media", CreatedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveNetworkShare() error = %v", err)
	}

	got := rt.doctorCheckNASClientTools(ctx)
	if len(got) != 1 {
		t.Fatalf("doctorCheckNASClientTools() = %+v, want exactly one check for the one configured nfs share", got)
	}
	if got[0].Code != "nas_mount.nfs" {
		t.Errorf("Code = %q, want %q", got[0].Code, "nas_mount.nfs")
	}
}

func TestDoctorCheckNASClientTools_BothProtocolsConfigured(t *testing.T) {
	rt, db := newTestRouterWithNetworkShareSecrets(t, &fakeNetworkShareSecretsSetter{})
	ctx := context.Background()

	if err := db.SaveNetworkShare(ctx, store.NetworkShare{
		ID: "ns_1", Name: "media-nas", Protocol: store.NetworkShareProtocolNFS,
		Host: "nas.lan", RemotePath: "/export/media", CreatedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveNetworkShare(nfs) error = %v", err)
	}
	if err := db.SaveNetworkShare(ctx, store.NetworkShare{
		ID: "ns_2", Name: "backup-nas", Protocol: store.NetworkShareProtocolCIFS,
		Host: "nas.lan", RemotePath: "/backups", Username: "u", CreatedAt: "2026-10-01T00:00:01Z",
	}); err != nil {
		t.Fatalf("SaveNetworkShare(cifs) error = %v", err)
	}

	got := rt.doctorCheckNASClientTools(ctx)
	if len(got) != 2 {
		t.Fatalf("doctorCheckNASClientTools() = %+v, want one check per distinct protocol configured", got)
	}
}
