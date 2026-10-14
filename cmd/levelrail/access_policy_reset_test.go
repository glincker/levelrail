package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAccessPolicyReset(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSessionPolicy(ctx, store.SessionPolicy{TeamID: store.DefaultTeamID, IPAllowlist: []string{"10.0.0.0/8"}, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	open := func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, dbPath) }

	var out bytes.Buffer
	if err := runAccessPolicyReset(ctx, nil, &out, open); err == nil {
		t.Fatal("reset ran without --yes")
	}
	if err := runAccessPolicyReset(ctx, []string{"--yes"}, &out, open); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "removed 1 session policies") {
		t.Fatalf("output %q", out.String())
	}
	db, err = open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if ps, _ := db.ListSessionPolicies(ctx); len(ps) != 0 {
		t.Fatalf("policies left: %+v", ps)
	}
}
