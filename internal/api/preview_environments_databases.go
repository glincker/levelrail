package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ephemeralDatabaseName derives the desired_databases.name a preview's
// own instance of sourceKey (an app.yaml databases: key) reconciles
// against. Deterministic and collision-free the same way previewAppName
// itself is guaranteed to be (previewName is already unique per app/PR,
// sourceKey is unique within one app.yaml's databases: map).
func ephemeralDatabaseName(previewName, sourceKey string) string {
	return previewName + "-db-" + sourceKey
}

// provisionEphemeralDatabases provisions one database.Controller-managed
// instance per databases entry with EphemeralInPreviews set, reusing the
// exact same SaveDesiredDatabase call POST /api/v1/databases
// (createDesiredDatabase, databases.go) makes: this function only picks
// the name/engine/version and records the linkage, the reconcile loop's
// own dynamicSource (cmd/levelrail/main.go) already lists every
// desired_databases row on every pass and reconciles it, so a freshly
// saved row here needs no further wiring to actually become a running
// container.
//
// Idempotent and safe to call on every deploy for the same pull request
// (a synchronize push re-runs this exactly like it re-runs the rest of
// deployPreviewEnvironment): a tracking row already present for
// (previewEnvironmentID, sourceKey) short-circuits, so a redeploy never
// mints a second database or loses track of the first one, including
// after a crash between saving the desired database and saving its own
// tracking row (the next call finds no tracking row and safely re-saves
// the desired database, itself an upsert). Best-effort per entry: one
// database failing to provision is logged and skipped, never fails the
// whole preview deploy, matching ensurePreviewEnvironmentTier's own
// "one broken resource must not block others" reasoning.
//
// Returns every database this call provisioned or already owned, so the
// caller can decide whether exactly one exists and is eligible for
// automatic env attachment (deployPreviewSingle's own use of this).
func (rt *Router) provisionEphemeralDatabases(ctx context.Context, previewEnvironmentID, previewName string, databases map[string]spec.Database) []store.PreviewEphemeralDatabase {
	var provisioned []store.PreviewEphemeralDatabase
	for key, d := range databases {
		if !d.EphemeralInPreviews {
			continue
		}
		p, err := rt.provisionOneEphemeralDatabase(ctx, previewEnvironmentID, previewName, key, d)
		if err != nil {
			rt.logger.Error("api: provision ephemeral preview database failed",
				slog.String("error", err.Error()),
				slog.String("preview_environment_id", previewEnvironmentID),
				slog.String("source_key", key))
			continue
		}
		provisioned = append(provisioned, p)
	}
	return provisioned
}

func (rt *Router) provisionOneEphemeralDatabase(ctx context.Context, previewEnvironmentID, previewName, sourceKey string, d spec.Database) (store.PreviewEphemeralDatabase, error) {
	existing, err := rt.previewEnvironments.GetPreviewEphemeralDatabaseByPreviewAndKey(ctx, previewEnvironmentID, sourceKey)
	if err == nil {
		return *existing, nil
	}
	if !errors.Is(err, store.ErrPreviewEphemeralDatabaseNotFound) {
		return store.PreviewEphemeralDatabase{}, fmt.Errorf("check existing ephemeral database: %w", err)
	}

	version := d.Version
	if version == "" {
		version = "latest"
	}

	dbName := ephemeralDatabaseName(previewName, sourceKey)
	if err := rt.databases.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: dbName, Engine: d.Engine, Version: version}); err != nil {
		return store.PreviewEphemeralDatabase{}, fmt.Errorf("save desired database %q: %w", dbName, err)
	}

	id, err := store.NewPreviewEphemeralDatabaseID()
	if err != nil {
		return store.PreviewEphemeralDatabase{}, fmt.Errorf("mint ephemeral database id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	p := store.PreviewEphemeralDatabase{
		ID:                   id,
		PreviewEnvironmentID: previewEnvironmentID,
		DatabaseName:         dbName,
		SourceKey:            sourceKey,
		Engine:               d.Engine,
		Version:              version,
		Status:               store.PreviewEphemeralDatabaseStatusProvisioned,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := rt.previewEnvironments.SavePreviewEphemeralDatabase(ctx, p); err != nil {
		return store.PreviewEphemeralDatabase{}, fmt.Errorf("save ephemeral database tracking row: %w", err)
	}
	return p, nil
}

// attachEphemeralDatabase points previewName's own DesiredService at
// db's connection string (the same PUT /api/v1/apps/{name}/database
// mechanism, store.DatabaseAttachment/UpdateServiceDatabaseAttachment,
// an operator would use by hand), then bumps its restart nonce
// (RestartService) so the next reconcile pass recreates its container
// with the new env rather than waiting for an unrelated future deploy to
// do so, matching this codebase's existing "an attachment change applies
// to freshly (re)created containers" contract (handleClearAppDatabase's
// own doc comment) rather than leaving it stuck until the next commit.
//
// Only called for a single-service preview with exactly one ephemeral
// database (deployPreviewSingle): store.DatabaseAttachment carries one
// connection per service, so a preview with more than one ephemeral
// database, or a multi-service fan-out where it's ambiguous which
// sibling should own the attachment, is left for an operator to wire by
// hand, the same as any other database attachment on any other app.
// Logged and non-fatal: this is a convenience on top of a working
// preview, not a precondition for one.
func (rt *Router) attachEphemeralDatabase(ctx context.Context, previewName string, db store.PreviewEphemeralDatabase) {
	att := &store.DatabaseAttachment{
		DatabaseName: db.DatabaseName,
		EnvVar:       defaultDatabaseAttachmentEnvVar,
		Field:        defaultDatabaseAttachmentField,
	}
	if !database.SupportsField(db.Engine, att.Field) {
		return
	}
	if err := rt.apps.UpdateServiceDatabaseAttachment(ctx, previewName, att); err != nil {
		rt.logger.Error("api: attach ephemeral preview database failed", slog.String("error", err.Error()), slog.String("preview_app", previewName), slog.String("database", db.DatabaseName))
		return
	}
	if err := rt.apps.RestartService(ctx, previewName); err != nil {
		rt.logger.Error("api: restart preview after database attachment failed", slog.String("error", err.Error()), slog.String("preview_app", previewName))
	}
}

// teardownPreviewEphemeralDatabases tears down every ephemeral database
// preview_environments row previewEnvironmentID owns: stops and removes
// its container (database.Controller.Teardown, resolved against the
// local runtime the same way ordinary managed databases run today; see
// ephemeralDatabaseName's own doc comment, previews have no node
// placement of their own yet), deletes its desired_databases row, then
// its own tracking row, in that order so a crash always leaves something
// concrete behind for a retry to find: the tracking row survives (with
// status teardown_failed and a reason) until every step has actually
// succeeded, mirroring teardownPreviewRecord's identical ordering for
// preview_environments itself.
//
// Synchronous, unlike teardownServiceContainers' fire-and-forget
// goroutine for an ordinary app's containers: a database holds data an
// operator was promised gets destroyed with no recovery path, so its
// removal must be a real, observable, retryable outcome, not a
// best-effort background cleanup an operator has no way to check on.
//
// Returns the database_name of every ephemeral database left undeleted,
// for teardownPreviewRecord to fold into its own partial-failure
// reporting; nil means every ephemeral database (zero or more) was fully
// torn down.
func (rt *Router) teardownPreviewEphemeralDatabases(ctx context.Context, previewEnvironmentID string) []string {
	dbs, err := rt.previewEnvironments.ListPreviewEphemeralDatabasesByPreview(ctx, previewEnvironmentID)
	if err != nil {
		rt.logger.Error("api: teardown ephemeral preview databases: list failed", slog.String("error", err.Error()), slog.String("preview_environment_id", previewEnvironmentID))
		return []string{previewEnvironmentID + ":list-failed"}
	}

	var failed []string
	for _, d := range dbs {
		if err := rt.teardownOneEphemeralDatabase(ctx, d); err != nil {
			rt.logger.Error("api: teardown ephemeral preview database failed", slog.String("error", err.Error()), slog.String("database", d.DatabaseName))
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if updErr := rt.previewEnvironments.UpdatePreviewEphemeralDatabaseStatus(ctx, d.ID, store.PreviewEphemeralDatabaseStatusTeardownFailed, err.Error(), now); updErr != nil {
				rt.logger.Error("api: record ephemeral preview database teardown failure failed", slog.String("error", updErr.Error()), slog.String("database", d.DatabaseName))
			}
			failed = append(failed, d.DatabaseName)
			continue
		}
	}
	return failed
}

// teardownOneEphemeralDatabase removes d's container (if the runtime is
// configured at all; nil execRuntime is a valid "not configured on this
// control plane" state, the same tolerance teardownServiceContainers
// itself already has), then its desired_databases row, then its own
// tracking row. Idempotent: a database already gone at any of these
// three steps (container never created, desired_databases row already
// deleted, tracking row already deleted) is treated as success, not a
// failure, so a retried teardown always converges instead of getting
// stuck re-reporting the same already-finished step.
func (rt *Router) teardownOneEphemeralDatabase(ctx context.Context, d store.PreviewEphemeralDatabase) error {
	if rt.execRuntime != nil {
		runtime, err := rt.execRuntime("")
		if err != nil {
			return fmt.Errorf("resolve runtime: %w", err)
		}
		if err := database.New(d.DatabaseName, nil, runtime).Teardown(ctx); err != nil {
			return fmt.Errorf("remove container: %w", err)
		}
	}

	if err := rt.databases.DeleteDesiredDatabase(ctx, d.DatabaseName); err != nil && !errors.Is(err, store.ErrDatabaseNotFound) {
		return fmt.Errorf("delete desired database: %w", err)
	}

	if err := rt.previewEnvironments.DeletePreviewEphemeralDatabase(ctx, d.ID); err != nil && !errors.Is(err, store.ErrPreviewEphemeralDatabaseNotFound) {
		return fmt.Errorf("delete tracking row: %w", err)
	}
	return nil
}
