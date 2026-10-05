package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const totpSecretEnvKeyName = "secret"

// authEngineOptions mounts the library auth beside the legacy routes when
// APP_AUTH_ENGINE=library. Off (the default) it returns nothing.
func authEngineOptions(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *sql.DB, mgr *secrets.Manager) []api.Option {
	if !authengine.Enabled() {
		return nil
	}
	cfg := authengine.Config{
		TokenPrefix: b.ShortName,
		TOTPIssuer:  b.Name,
		Directory:   authengine.NewDirectory(db),
	}
	if dial := dashboardDialAddr(httpAddr()); dial != "" {
		cfg.BaseURL = "http://" + dial
	}
	if mgr != nil {
		key, err := authengine.LoadOrCreateKey(ctx, mgr, true)
		if err != nil {
			logger.Error("auth engine: encryption key unavailable, TOTP stays off", slog.String("error", err.Error()))
		} else {
			cfg.EncryptionKey = key
		}
	}
	eng, err := authengine.New(db, authengine.ConfigFromEnv(cfg))
	if err != nil {
		logger.Error("auth engine: setup failed, library routes stay off", slog.String("error", err.Error()))
		return nil
	}
	logger.Info("auth engine: library routes mounted", slog.String("prefix", eng.Prefix()))
	return []api.Option{api.WithAuthEngine(eng.Prefix(), eng.Handler())}
}

// runAuthBackfill implements `<binary> auth-backfill [--dry-run]`.
func runAuthBackfill(ctx context.Context, args []string, dataDir string, stdout io.Writer) error {
	name := filepath.Base(os.Args[0])
	fs := flag.NewFlagSet(name+" auth-backfill", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "report what would be copied, change nothing")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse auth-backfill args: %w", err)
	}
	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = db.Close() }()
	return backfillAuth(ctx, db, dataDir, *dryRun, stdout)
}

func backfillAuth(ctx context.Context, db *store.DB, dataDir string, dryRun bool, stdout io.Writer) error {
	mgr, _, err := loadSecretsManager(db, dataDir)
	if err != nil {
		return fmt.Errorf("load secrets manager: %w", err)
	}
	key, err := authengine.LoadOrCreateKey(ctx, mgr, !dryRun)
	if err != nil {
		return err
	}
	rep, err := authengine.Backfill(ctx, db.DB, authengine.BackfillOptions{
		DryRun:            dryRun,
		Secrets:           mgr,
		EncryptionKey:     key,
		TOTPSecretService: store.UserTOTPSecretsKey,
		TOTPSecretKey:     totpSecretEnvKeyName,
	})
	if err != nil {
		return fmt.Errorf("auth backfill: %w", err)
	}
	mode := "applied"
	if dryRun {
		mode = "dry run, nothing written"
	}
	_, _ = fmt.Fprintf(stdout, "auth backfill (%s)\n", mode)
	_, _ = fmt.Fprintf(stdout, "  users copied:            %d (already mapped: %d)\n", rep.Users, rep.UsersAlreadyMapped)
	_, _ = fmt.Fprintf(stdout, "  password hashes copied:  %d\n", rep.Passwords)
	_, _ = fmt.Fprintf(stdout, "  api tokens copied:       %d (skipped, no owner: %d)\n", rep.Tokens, rep.TokensSkippedNoOwner)
	_, _ = fmt.Fprintf(stdout, "  passkeys copied:         %d\n", rep.Passkeys)
	_, _ = fmt.Fprintf(stdout, "  totp secrets copied:     %d\n", rep.TOTP)
	if rep.RecoveryCodesNotMoved > 0 {
		_, _ = fmt.Fprintf(stdout, "  note: recovery codes are not converted; %d user(s) must regenerate them\n", rep.RecoveryCodesNotMoved)
	}
	return nil
}
