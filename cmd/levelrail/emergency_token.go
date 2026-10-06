package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envEmergencyMaxTTL     = "APP_EMERGENCY_TOKEN_MAX_TTL"
	defaultEmergencyTTL    = time.Hour
	defaultEmergencyMaxTTL = 24 * time.Hour
	minEmergencyTTL        = time.Minute
	emergencyTokenName     = "emergency (host)"
	emergencyTokenPrefix   = "emergency"
)

// parseEmergencyTokenArgs validates the flags. The lifetime is capped so a
// forgotten emergency token cannot become a permanent root credential.
func parseEmergencyTokenArgs(args []string, lookupEnv func(string) (string, bool)) (username string, ttl time.Duration, err error) {
	fs := flag.NewFlagSet("emergency-token", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&username, "username", "", "admin account that owns the token (default: the first admin)")
	fs.DurationVar(&ttl, "ttl", defaultEmergencyTTL, "how long the token works, for example 30m or 4h")
	if err := fs.Parse(args); err != nil {
		return "", 0, fmt.Errorf("parse emergency-token args: %w", err)
	}
	maxTTL := defaultEmergencyMaxTTL
	if raw, ok := lookupEnv(envEmergencyMaxTTL); ok && raw != "" {
		d, perr := time.ParseDuration(raw)
		if perr != nil || d <= 0 {
			return "", 0, fmt.Errorf("%s %q is not a positive duration", envEmergencyMaxTTL, raw)
		}
		maxTTL = d
	}
	if ttl < minEmergencyTTL || ttl > maxTTL {
		return "", 0, fmt.Errorf("--ttl must be between %s and %s", minEmergencyTTL, maxTTL)
	}
	return strings.TrimSpace(username), ttl, nil
}

// pickEmergencyOwner finds the account the token acts as: the named one, or
// the first user holding the root ability.
func pickEmergencyOwner(ctx context.Context, db *store.DB, username string) (*store.User, error) {
	if username != "" {
		u, err := db.GetUserByEmail(ctx, username)
		if err != nil {
			return nil, fmt.Errorf("load user %q: %w", username, err)
		}
		return u, nil
	}
	users, err := db.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	for i := range users {
		if slices.Contains(users[i].Abilities, api.AbilityRoot) {
			return &users[i], nil
		}
	}
	return nil, errors.New("no admin account exists; run recover-admin first")
}

func randomEmergencyID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// runEmergencyToken implements `<binary> emergency-token [--username U] [--ttl 1h]`.
// It needs the same host access as recover-admin (the data directory), mints a
// short-lived root API token through the auth engine, mirrors it into the
// platform table so it is listed and revocable like any other, and records an
// audit entry. The secret is printed once and never stored in plaintext.
func runEmergencyToken(ctx context.Context, logger *slog.Logger, args []string, stdout io.Writer, openStore func(context.Context) (*store.DB, error)) error {
	username, ttl, err := parseEmergencyTokenArgs(args, os.LookupEnv)
	if err != nil {
		return err
	}
	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = db.Close() }()

	owner, err := pickEmergencyOwner(ctx, db, username)
	if err != nil {
		return err
	}
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL:     "http://localhost",
		TokenPrefix: emergencyTokenPrefix,
		Directory:   authengine.NewDirectory(db.DB),
	})
	if err != nil {
		return fmt.Errorf("start auth engine: %w", err)
	}

	expires := time.Now().UTC().Add(ttl)
	abilities := []string{api.AbilityRoot}
	raw, rec, err := eng.MintToken(ctx, authengine.MintInput{
		OwnerLegacyID: owner.ID, Name: emergencyTokenName, Abilities: abilities, ExpiresAt: &expires,
	})
	if err != nil {
		return fmt.Errorf("mint emergency token: %w", err)
	}
	legacyID, err := randomEmergencyID("tok_")
	if err != nil {
		return err
	}
	mirror := store.APIToken{
		ID: legacyID, Name: rec.Name, TokenHash: rec.HashHex, Abilities: abilities,
		CreatedAt: rec.CreatedAt, ExpiresAt: &expires, OwnerUserID: owner.ID,
	}
	if err := db.SaveAPIToken(ctx, mirror); err != nil {
		_ = eng.RevokeToken(ctx, rec.EngineID)
		return fmt.Errorf("save emergency token: %w", err)
	}
	if err := eng.LinkToken(ctx, legacyID, rec.EngineID); err != nil {
		return fmt.Errorf("link emergency token: %w", err)
	}
	auditID, err := randomEmergencyID("aud_")
	if err != nil {
		return err
	}
	if err := db.SaveAuditEntry(ctx, store.AuditEntry{
		ID: auditID, ActorType: "system", ActorID: "host", ActorName: "emergency-token",
		Ability: api.AbilityRoot, Method: "CLI", Path: "emergency-token " + owner.Email,
		StatusCode: 200, RemoteAddr: "host", CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientKind: "system",
	}); err != nil {
		logger.Error("emergency token audit entry failed", slog.String("error", err.Error()))
	}
	logger.Warn("emergency token minted", slog.String("owner", owner.Email), slog.String("token_id", legacyID), slog.Time("expires_at", expires))

	_, _ = fmt.Fprintf(stdout, "emergency token for %q (root), valid until %s:\n\n  %s\n\n", owner.Email, expires.Format(time.RFC3339), raw)
	_, _ = fmt.Fprintln(stdout, "shown once. Use it with the CLI: APP_API_TOKEN=<token> levelrail-cli <command>")
	_, _ = fmt.Fprintln(stdout, "It expires on its own; revoke it earlier under Settings, CLI Access.")
	return nil
}
