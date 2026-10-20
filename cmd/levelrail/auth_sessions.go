package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// authSessionsHooks wires the sessions area's host callbacks: library auth
// events land in the audit log and library mail goes out through the router.
func authSessionsHooks(db *sql.DB, logger *slog.Logger) authengine.SessionsHooks {
	audit := &store.DB{DB: db}
	return authengine.SessionsHooks{
		Mail: &authengine.MailRelay{},
		Audit: func(ctx context.Context, rec authengine.AuditRecord) {
			id, err := store.NewAuditEntryID()
			if err != nil {
				logger.Warn("auth sessions: audit id failed", slog.String("error", err.Error()))
				return
			}
			entry := store.AuditEntry{
				ID: id, ActorType: rec.ActorType, ActorID: rec.ActorID, ActorName: rec.ActorName,
				Ability: rec.Ability, Method: rec.Method, Path: rec.Path, StatusCode: rec.StatusCode,
				RemoteAddr: rec.RemoteAddr, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "dashboard",
			}
			if err := audit.SaveAuditEntry(ctx, entry); err != nil {
				logger.Warn("auth sessions: save audit entry failed", slog.String("error", err.Error()))
			}
		},
	}
}

// recoverAdminLibrary mirrors a recover-admin password reset into the library,
// which also revokes sessions and clears lockouts.
func recoverAdminLibrary(ctx context.Context, db *store.DB, username, password string, stdout io.Writer) error {
	user, err := db.GetUserByEmail(ctx, username)
	if err != nil {
		return fmt.Errorf("load recovered user: %w", err)
	}
	if err := authengine.RecoverAdmin(ctx, db.DB, toLibraryUser(*user), password); err != nil {
		return fmt.Errorf("recover admin in auth library: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, "sessions and login lockouts for this account were cleared.")
	return nil
}

func toLibraryUser(u store.User) authengine.LegacyUser {
	lu := authengine.LegacyUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt}
	if u.PasswordHash != nil {
		lu.PasswordHash = *u.PasswordHash
	}
	return lu
}
