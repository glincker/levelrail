package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSecurityResetApprovalPolicy(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	hold := "hold"
	if err := db.SaveSecurityPolicy(ctx, store.SecurityPolicy{ApprovalWithoutSession: &hold, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	open := func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, dbPath) }
	var out bytes.Buffer
	if err := runSecurity(ctx, []string{"other"}, &out, open); err == nil {
		t.Fatal("an unknown subcommand must fail")
	}
	if err := runSecurity(ctx, []string{"reset-approval-policy"}, &out, open); err != nil {
		t.Fatal(err)
	}
	db, err = open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	p, _, err := db.GetSecurityPolicy(ctx)
	if err != nil || p.ApprovalWithoutSession == nil || *p.ApprovalWithoutSession != approvalAllow {
		t.Fatalf("policy after reset = %+v, %v", p.ApprovalWithoutSession, err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE actor_name = 'security reset-approval-policy'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit rows = %d, %v", n, err)
	}
}
