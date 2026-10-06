package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

func noEnv(string) (string, bool) { return "", false }

func TestParseEmergencyTokenArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantTTL time.Duration
		wantErr bool
	}{
		{"defaults to one hour", nil, nil, time.Hour, false},
		{"explicit ttl", []string{"--ttl", "30m"}, nil, 30 * time.Minute, false},
		{"too short", []string{"--ttl", "10s"}, nil, 0, true},
		{"over the default cap", []string{"--ttl", "48h"}, nil, 0, true},
		{"cap can be raised by the operator", []string{"--ttl", "48h"}, map[string]string{envEmergencyMaxTTL: "72h"}, 48 * time.Hour, false},
		{"bad cap is rejected", nil, map[string]string{envEmergencyMaxTTL: "soon"}, 0, true},
		{"unknown flag", []string{"--forever"}, nil, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok }
			_, ttl, err := parseEmergencyTokenArgs(tc.args, lookup)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && ttl != tc.wantTTL {
				t.Errorf("ttl = %v, want %v", ttl, tc.wantTTL)
			}
		})
	}
}

func TestRunEmergencyToken_MintsShortLivedRootTokenThatAuthenticates(t *testing.T) {
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

	var stdout bytes.Buffer
	if err := runEmergencyToken(ctx, logger, []string{"--ttl", "30m"}, &stdout, open); err != nil {
		t.Fatalf("runEmergencyToken() error = %v", err)
	}
	var raw string
	for _, f := range strings.Fields(stdout.String()) {
		if strings.HasPrefix(f, emergencyTokenPrefix+"_") {
			raw = f
		}
	}
	if raw == "" {
		t.Fatalf("stdout = %q, want the token printed once", stdout.String())
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
	rec, err := eng.LookupBearer(ctx, raw)
	if err != nil {
		t.Fatalf("the printed token does not authenticate: %v", err)
	}
	if rec.ExpiresAt == nil || time.Until(*rec.ExpiresAt) > 31*time.Minute || time.Until(*rec.ExpiresAt) < 25*time.Minute {
		t.Errorf("ExpiresAt = %v, want about 30 minutes from now", rec.ExpiresAt)
	}

	tokens, err := db.ListAPITokens(ctx)
	if err != nil || len(tokens) != 1 || tokens[0].Name != emergencyTokenName || tokens[0].ExpiresAt == nil {
		t.Errorf("platform tokens = %+v (err %v), want one expiring %q token so it is listed and revocable", tokens, err, emergencyTokenName)
	}
	if strings.Contains(strings.Join([]string{tokens[0].TokenHash}, ""), raw) {
		t.Error("the plaintext token must never be stored")
	}
	entries, err := db.ListAuditEntries(ctx, 10, nil, store.AuditEntryFilter{})
	if err != nil || len(entries) == 0 || entries[0].ActorName != "emergency-token" || entries[0].Ability != api.AbilityRoot {
		t.Errorf("audit entries = %+v (err %v), want the mint recorded", entries, err)
	}
}

func TestRunEmergencyToken_RefusesWithoutAnAdmin(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "levelrail.db")
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	var stdout bytes.Buffer
	err := runEmergencyToken(ctx, logger, nil, &stdout, func(ctx context.Context) (*store.DB, error) { return store.Open(ctx, dbPath) })
	if err == nil || !strings.Contains(err.Error(), "no admin account") {
		t.Fatalf("error = %v, want a refusal when no admin exists", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, must print no token on failure", stdout.String())
	}
	_ = os.Remove(dbPath)
}
