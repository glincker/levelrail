package main

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRunApproveSignIn(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "levelrail.db")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	open := func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, dbPath) }

	seed, err := open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := recoverAdminUser(ctx, seed, "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	owner, err := pickEmergencyOwner(ctx, seed, "admin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, id := range []string{"la_right", "la_wrong"} {
		if err := seed.CreateLoginApproval(ctx, store.LoginApproval{
			ID: id, UserID: owner.ID, BrowserHash: "hash-" + id, RequesterIP: "203.0.113.9", UserAgent: "Firefox on macOS",
			CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	_ = seed.Close()

	var out bytes.Buffer
	if err := runApproveSignIn(ctx, logger, []string{"la_right", "--match", itoa(api.ApprovalMatch("hash-la_right"))}, &out, open); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := runApproveSignIn(ctx, logger, []string{"la_wrong", "--match", itoa(api.ApprovalMatch("hash-la_wrong") + 1)}, &out, open); err == nil {
		t.Fatal("a wrong number must not approve")
	}

	db, err := open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	got, _ := db.GetLoginApproval(ctx, "la_right")
	if got.Status != store.LoginApprovalApproved {
		t.Errorf("right request status = %q, want approved", got.Status)
	}
	bad, _ := db.GetLoginApproval(ctx, "la_wrong")
	if bad.Status == store.LoginApprovalApproved {
		t.Error("the mismatched request was approved")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
