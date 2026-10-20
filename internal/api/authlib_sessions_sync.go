package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

type libSyncSkipKey struct{}

// withoutLibSync marks a user-store write whose library side the caller
// already handled, so the sync wrapper does not repeat or audit it.
func withoutLibSync(ctx context.Context) context.Context {
	return context.WithValue(ctx, libSyncSkipKey{}, true)
}

func libSyncSkipped(ctx context.Context) bool {
	skip, _ := ctx.Value(libSyncSkipKey{}).(bool)
	return skip
}

// libSyncedAuth keeps the library user table consistent with the platform's
// own users: every create, password change and delete funnels through the
// AuthStore, so one wrapper covers invites, admin create, OAuth and reset.
type libSyncedAuth struct {
	AuthStore
	sess   *authengine.Sessions
	logger *slog.Logger
}

func (a *libSyncedAuth) CreateUser(ctx context.Context, u store.User) error {
	if err := a.AuthStore.CreateUser(ctx, u); err != nil {
		return err
	}
	if libSyncSkipped(ctx) {
		return nil
	}
	if _, err := a.sess.SyncUser(ctx, toLibUser(u)); err != nil {
		a.logger.Warn("api: sync new user to auth library failed, will retry at sign-in", slog.String("error", err.Error()), slog.String("user_id", u.ID))
	}
	return nil
}

func (a *libSyncedAuth) UpdateUserPasswordHash(ctx context.Context, id string, hash *string) error {
	if err := a.AuthStore.UpdateUserPasswordHash(ctx, id, hash); err != nil {
		return err
	}
	if libSyncSkipped(ctx) {
		return nil
	}
	next := ""
	if hash != nil {
		next = *hash
	}
	engineID, err := a.sess.SyncPassword(ctx, id, next)
	if err != nil {
		a.logger.Warn("api: sync password hash to auth library failed", slog.String("error", err.Error()), slog.String("user_id", id))
		return nil
	}
	a.sess.EmitPasswordChanged(ctx, engineID)
	return nil
}

func (a *libSyncedAuth) DeleteUser(ctx context.Context, id string) error {
	engineID, lookupErr := a.sess.EngineIDFor(ctx, id)
	if err := a.AuthStore.DeleteUser(ctx, id); err != nil {
		return err
	}
	if lookupErr != nil {
		a.logger.Warn("api: look up library user before delete failed", slog.String("error", lookupErr.Error()), slog.String("user_id", id))
		return nil
	}
	if err := a.sess.DeleteUser(ctx, engineID); err != nil {
		a.logger.Warn("api: delete library user failed", slog.String("error", err.Error()), slog.String("user_id", id))
	}
	return nil
}
