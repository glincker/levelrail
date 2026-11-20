package store

import (
	"context"
	"strings"
	"testing"
)

func TestBackupLabelsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetBackupLabel(ctx, "b1", "before upgrade"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetBackupLabel(ctx, "b2", "x"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetBackupLabel(ctx, "b2", ""); err != nil {
		t.Fatal(err)
	}
	got, err := db.BackupLabels(ctx, []string{"b1", "b2", "b3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["b1"] != "before upgrade" {
		t.Fatalf("labels = %v", got)
	}
}

func TestNormalizeBackupLabel(t *testing.T) {
	if v, err := NormalizeBackupLabel("  ok "); err != nil || v != "ok" {
		t.Fatalf("got %q %v", v, err)
	}
	if _, err := NormalizeBackupLabel(strings.Repeat("a", MaxBackupLabelLen+1)); err == nil {
		t.Fatal("expected too long error")
	}
	if _, err := NormalizeBackupLabel("a\nb"); err == nil {
		t.Fatal("expected control char error")
	}
}
