package main

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRunEmergencyToken_RevokesOldestBeyondTheCap(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "levelrail.db")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	open := func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, dbPath) }
	seed, err := open(ctx)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := recoverAdminUser(ctx, seed, "admin", "correct horse battery staple"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	_ = seed.Close()

	var last bytes.Buffer
	for i := 0; i < 5; i++ {
		last.Reset()
		if err := runEmergencyToken(ctx, logger, nil, &last, open); err != nil {
			t.Fatalf("mint %d: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(last.String(), "revoked 1 older emergency tokens") {
		t.Errorf("output = %q, want the revoked count printed", last.String())
	}
	db, err := open(ctx)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	eng, err := authengine.New(db.DB, authengine.Config{BaseURL: "http://localhost", TokenPrefix: emergencyTokenPrefix, Directory: authengine.NewDirectory(db.DB)})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	recs, err := eng.ListTokens(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	live := 0
	for _, r := range recs {
		if r.RevokedAt == nil {
			live++
		}
	}
	if live != 3 || len(recs) != 5 {
		t.Errorf("live = %d of %d, want 3 live of 5 (default cap keeps the audit trail)", live, len(recs))
	}
}
