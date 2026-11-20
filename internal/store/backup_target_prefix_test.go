package store

import (
	"context"
	"testing"
)

// The bucket field carries the operator's folder or prefix, so a rename or region edit must never rewrite it.
func TestUpdateBackupTargetKeepsPathPrefix(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	target := newTestBackupTarget()
	target.Bucket = "levelrail-backups/prod/databases"
	if err := db.SaveBackupTarget(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateBackupTarget(ctx, target.ID, "renamed", target.Provider, target.Endpoint, "eu-1", target.Bucket); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetBackupTarget(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bucket != "levelrail-backups/prod/databases" || got.Name != "renamed" || got.Region != "eu-1" {
		t.Fatalf("target after update = %+v", got)
	}
}
