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
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/store"
)

const defaultRecoverAdminUsername = "admin"

// parseRecoverAdminArgs parses `levelrail recover-admin` flags. An empty
// returned password means the caller should generate one: there is no
// sentinel string for "generate," an empty flag value already means that.
func parseRecoverAdminArgs(args []string) (username, password string, err error) {
	fs := flag.NewFlagSet("recover-admin", flag.ContinueOnError)
	usernameFlag := fs.String("username", defaultRecoverAdminUsername, "admin username to set")
	passwordFlag := fs.String("password", "", "admin password to set (generated if omitted)")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	return *usernameFlag, *passwordFlag, nil
}

// generateRandomPassword returns a URL-safe, high-entropy password for the
// case where the operator running recover-admin doesn't supply one. 24
// random bytes, base64-encoded, comfortably exceeds any bcrypt-relevant
// entropy floor and prints cleanly in a terminal.
func generateRandomPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// recoverAdminUser bcrypt-hashes password and writes it directly against
// the users table, bypassing the HTTP API. username is the login
// identifier: this resets the password if a matching user already
// exists, or creates a new, ordinary (not first-user) account if not,
// so the operator doesn't need to know in advance which case applies.
func recoverAdminUser(ctx context.Context, db *store.DB, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	hashStr := string(hash)

	existing, err := db.GetUserByEmail(ctx, username)
	if err == nil {
		if err := db.UpdateUserPasswordHash(ctx, existing.ID, &hashStr); err != nil {
			return fmt.Errorf("update user password: %w", err)
		}
		return nil
	}
	if !errors.Is(err, store.ErrUserNotFound) {
		return fmt.Errorf("look up existing user: %w", err)
	}

	id, err := newRecoveredUserID()
	if err != nil {
		return fmt.Errorf("generate user id: %w", err)
	}
	user := store.User{
		ID:           id,
		Email:        username,
		DisplayName:  username,
		PasswordHash: &hashStr,
		CreatedAt:    time.Now(),
	}
	if err := db.CreateUser(ctx, user); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// newRecoveredUserID mints an opaque ID matching internal/api's own
// randomOpaqueID shape, duplicated here since that helper isn't exported.
func newRecoveredUserID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "user_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// runRecoverAdmin is the recover-admin subcommand's entry point, taking
// openStore as a parameter (rather than calling the package-level
// openStore directly) so tests can point it at an isolated store without
// touching APP_DATA_DIR or the filesystem.
//
// Known limitation, deliberately not worked around here: this cannot
// invalidate the running server's in-memory session map, since that map
// lives in a separate OS process. Restarting the container after recovery
// is the practical way to revoke sessions established under the old
// password.
func runRecoverAdmin(ctx context.Context, logger *slog.Logger, args []string, stdout io.Writer, openStore func(context.Context) (*store.DB, error)) error {
	username, password, err := parseRecoverAdminArgs(args)
	if err != nil {
		return fmt.Errorf("parse recover-admin args: %w", err)
	}

	generated := password == ""
	if generated {
		password, err = generateRandomPassword()
		if err != nil {
			return err
		}
	}

	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := recoverAdminUser(ctx, db, username, password); err != nil {
		return err
	}

	logger.Warn("admin account recovered",
		slog.String("username", username),
		slog.Bool("password_generated", generated),
	)

	// stdout write failures here are not actionable (the recovery itself
	// already succeeded and was logged above), so they're deliberately
	// swallowed rather than turning a successful recovery into a
	// nonzero exit code.
	_, _ = fmt.Fprintf(stdout, "admin account %q recovered.\n", username)
	if generated {
		_, _ = fmt.Fprintf(stdout, "generated password: %s\n", password)
	}
	_, _ = fmt.Fprintln(stdout, "restart the container to invalidate sessions created under the old password.")

	return nil
}
