package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/glincker/levelrail/internal/secrets"
	"github.com/glincker/levelrail/internal/store"
)

// AuditActionSecretNamespaceCleared is the audit event for a stale secret namespace wiped before a name was reused.
const AuditActionSecretNamespaceCleared = "secret.namespace_cleared"

// databaseUndoKept reports whether a soft database delete is remembered for undo.
func (rt *Router) databaseUndoKept(ctx context.Context) bool {
	return rt.recentlyDeleted() != nil && rt.recentlyDeletedWindow(ctx) > 0
}

// canParkSecrets reports whether the secrets manager can copy a namespace aside.
func (rt *Router) canParkSecrets() bool {
	_, ok := rt.secrets.(namespaceCopier)
	return ok
}

// parkDatabaseSecrets keeps a copy of the database credentials under its deletion namespace.
func (rt *Router) parkDatabaseSecrets(ctx context.Context, name string) error {
	c, ok := rt.secrets.(namespaceCopier)
	if !ok {
		return nil
	}
	if err := secrets.AppNamespace(name); err != nil {
		return err
	}
	ns := store.DeletedDatabaseSecretsKey(name)
	if err := rt.secrets.DeleteAll(ctx, ns); err != nil {
		return err
	}
	return c.CopyNamespace(ctx, name, ns)
}

// dropParkedDatabaseSecrets erases the credentials parked for an undo.
func (rt *Router) dropParkedDatabaseSecrets(ctx context.Context, name string) error {
	if rt.secrets == nil {
		return nil
	}
	return rt.secrets.DeleteAll(ctx, store.DeletedDatabaseSecretsKey(name))
}

// restoreParkedDatabaseSecrets puts parked credentials back under the live name. An entry
// without a parked copy (deleted before parking existed) keeps whatever the live name holds.
func (rt *Router) restoreParkedDatabaseSecrets(ctx context.Context, name string) error {
	c, ok := rt.secrets.(namespaceCopier)
	if !ok {
		return nil
	}
	if err := secrets.AppNamespace(name); err != nil {
		return err
	}
	parked := store.DeletedDatabaseSecretsKey(name)
	keys, err := rt.secrets.ListKeys(ctx, parked)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	if err := rt.secrets.DeleteAll(ctx, name); err != nil {
		return err
	}
	return c.CopyNamespace(ctx, parked, name)
}

// eraseRestoredDatabaseSecrets undoes restoreParkedDatabaseSecrets after a failed restore; the parked copy stays.
func (rt *Router) eraseRestoredDatabaseSecrets(ctx context.Context, name string) {
	if rt.secrets == nil || secrets.AppNamespace(name) != nil {
		return
	}
	bg := context.WithoutCancel(ctx)
	keys, err := rt.secrets.ListKeys(bg, store.DeletedDatabaseSecretsKey(name))
	if err != nil || len(keys) == 0 {
		return
	}
	if err := rt.secrets.DeleteAll(bg, name); err != nil {
		rt.logger.Warn("api: erase restored database credentials failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// dropOrphanParkedDatabase erases the parked copy when no undo entry was saved for it.
func (rt *Router) dropOrphanParkedDatabase(ctx context.Context, name string, parked bool) {
	st := rt.recentlyDeleted()
	if !parked || st == nil {
		return
	}
	if _, err := st.GetRecentlyDeleted(ctx, store.RecentlyDeletedDatabase, name); errors.Is(err, store.ErrRecentlyDeletedNotFound) {
		if derr := rt.dropParkedDatabaseSecrets(ctx, name); derr != nil {
			rt.logger.Warn("api: erase orphan parked credentials failed", slog.String("name", name), slog.String("error", derr.Error()))
		}
	}
}

// eraseDatabaseCredentials removes every value under the database's own namespace.
func (rt *Router) eraseDatabaseCredentials(ctx context.Context, name string) error {
	if rt.secrets == nil {
		return nil
	}
	if err := secrets.AppNamespace(name); err != nil {
		return err
	}
	if err := rt.secrets.DeleteAll(ctx, name); err != nil {
		return fmt.Errorf("erase credentials of %q: %w", name, err)
	}
	return nil
}

// clearStaleSecretNamespace wipes values left under a name that no app, database or stack holds,
// so the next owner never inherits them. The caller holds the name lock and has checked the name is free.
func (rt *Router) clearStaleSecretNamespace(ctx context.Context, name string) error {
	if rt.secrets == nil {
		return nil
	}
	// A control plane namespace is never wiped here.
	if err := secrets.AppNamespace(name); err != nil {
		return errAppNameTaken
	}
	keys, err := rt.secrets.ListKeys(ctx, name)
	if err != nil {
		return fmt.Errorf("check secret namespace %q: %w", name, err)
	}
	if err := rt.secrets.DeleteAll(ctx, name); err != nil {
		return fmt.Errorf("clear secret namespace %q: %w", name, err)
	}
	if len(keys) > 0 {
		rt.logger.Warn("api: stale secret namespace cleared", slog.String("name", name), slog.Int("keys", len(keys)))
		rt.auditStaleNamespace(ctx, name, len(keys))
	}
	return nil
}

func (rt *Router) auditStaleNamespace(ctx context.Context, name string, keys int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "control-plane", ActorName: "control plane",
		Ability: AbilityWrite, Method: "POST", Path: "/api/v1/secrets/namespace", StatusCode: 200,
		CreatedAt: store.FormatAuditTime(time.Now()),
		Action:    AuditActionSecretNamespaceCleared,
		Detail:    fmt.Sprintf(`{"name":%q,"keys":%d}`, name, keys),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: stale namespace audit save failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}
