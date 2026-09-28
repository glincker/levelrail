package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// isolatedRoleExecTimeout bounds the CREATE ROLE/GRANT exec against a
// live Postgres container: generous relative to how fast these
// statements normally run, short enough that a stuck container fails a
// preview deploy instead of hanging it.
const isolatedRoleExecTimeout = 15 * time.Second

// provisionDatabaseIsolations provisions an isolated Postgres role for
// every databases entry with IsolatedInPreviews set (and
// EphemeralInPreviews not set: that flag already gets its own instance
// with its own credentials, so isolation is redundant there). Best
// effort per entry, matching provisionEphemeralDatabases' own "one
// broken resource must not block others" reasoning: a failure is logged
// and skipped, never fails the whole preview deploy.
func (rt *Router) provisionDatabaseIsolations(ctx context.Context, previewEnvironmentID, previewName string, databases map[string]spec.Database) {
	for key, d := range databases {
		if d.EphemeralInPreviews || !d.IsolatedInPreviews {
			continue
		}
		if err := rt.provisionOneDatabaseIsolation(ctx, previewEnvironmentID, previewName, key); err != nil {
			rt.logger.Error("api: provision preview database isolation failed",
				slog.String("error", err.Error()),
				slog.String("preview_environment_id", previewEnvironmentID),
				slog.String("source_key", key))
		}
	}
}

func (rt *Router) provisionOneDatabaseIsolation(ctx context.Context, previewEnvironmentID, previewName, sourceKey string) error {
	if _, err := rt.previewEnvironments.GetPreviewDatabaseIsolationByPreviewAndKey(ctx, previewEnvironmentID, sourceKey); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrPreviewDatabaseIsolationNotFound) {
		return fmt.Errorf("check existing database isolation: %w", err)
	}

	desired, err := rt.databases.GetDesiredDatabase(ctx, sourceKey)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		return fmt.Errorf("database %q is not an existing managed database: %w", sourceKey, err)
	}
	if err != nil {
		return fmt.Errorf("load database %q: %w", sourceKey, err)
	}
	if desired.Engine != store.EnginePostgres {
		return fmt.Errorf("database %q: isolatedInPreviews only supports engine %q, got %q", sourceKey, store.EnginePostgres, desired.Engine)
	}
	if rt.execRuntime == nil || rt.secrets == nil {
		return fmt.Errorf("database %q: isolation needs both exec and secrets configured on this control plane", sourceKey)
	}

	role := database.IsolatedRoleName(previewName, sourceKey)
	password, err := database.GenerateIsolatedRolePassword()
	if err != nil {
		return fmt.Errorf("generate role password: %w", err)
	}

	if err := rt.execIsolatedRoleSQL(ctx, sourceKey, desired.NodeID, database.CreateIsolatedRoleSQL(role, desired.Name, password)); err != nil {
		return fmt.Errorf("create role %q on database %q: %w", role, sourceKey, err)
	}

	secretEnvKey := "password"
	secretService := previewDatabaseIsolationSecretService(previewEnvironmentID, sourceKey)
	if err := rt.secrets.SetValueGuarded(ctx, secretService, secretEnvKey, password, true); err != nil {
		return fmt.Errorf("store role credential: %w", err)
	}

	id, err := store.NewPreviewDatabaseIsolationID()
	if err != nil {
		return fmt.Errorf("mint database isolation id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	p := store.PreviewDatabaseIsolation{
		ID:                   id,
		PreviewEnvironmentID: previewEnvironmentID,
		DatabaseName:         sourceKey,
		SourceKey:            sourceKey,
		RoleName:             role,
		SecretEnvKey:         secretEnvKey,
		Status:               store.PreviewDatabaseIsolationStatusProvisioned,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := rt.previewEnvironments.SavePreviewDatabaseIsolation(ctx, p); err != nil {
		return fmt.Errorf("save database isolation tracking row: %w", err)
	}
	return nil
}

// teardownPreviewDatabaseIsolations drops every isolated role
// previewEnvironmentID owns, then deletes its own tracking row, in that
// order so a crash always leaves something concrete behind for a retry
// to find: the tracking row survives (with status teardown_failed and a
// reason) until the role is actually gone. Returns the role name of any
// isolation left undeleted, for teardownPreviewRecordReason to fold into
// its own partial-failure reporting; nil means every isolation (zero or
// more) was fully torn down.
func (rt *Router) teardownPreviewDatabaseIsolations(ctx context.Context, previewEnvironmentID string) []string {
	isolations, err := rt.previewEnvironments.ListPreviewDatabaseIsolationsByPreview(ctx, previewEnvironmentID)
	if err != nil {
		rt.logger.Error("api: teardown preview database isolations: list failed", slog.String("error", err.Error()), slog.String("preview_environment_id", previewEnvironmentID))
		return []string{previewEnvironmentID + ":list-failed"}
	}

	var failed []string
	for _, p := range isolations {
		if err := rt.teardownOneDatabaseIsolation(ctx, p); err != nil {
			rt.logger.Error("api: teardown preview database isolation failed", slog.String("error", err.Error()), slog.String("role", p.RoleName))
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if updErr := rt.previewEnvironments.UpdatePreviewDatabaseIsolationStatus(ctx, p.ID, store.PreviewDatabaseIsolationStatusTeardownFailed, err.Error(), now); updErr != nil {
				rt.logger.Error("api: record preview database isolation teardown failure failed", slog.String("error", updErr.Error()), slog.String("role", p.RoleName))
			}
			failed = append(failed, p.RoleName)
			continue
		}
	}
	return failed
}

// teardownOneDatabaseIsolation drops p's role (if the database it lived
// on still exists; one already deleted is treated as the role being
// gone too), deletes its secret, then its own tracking row. Idempotent:
// a database, role, or secret already gone at any step is success, not
// failure, so a retried teardown always converges.
func (rt *Router) teardownOneDatabaseIsolation(ctx context.Context, p store.PreviewDatabaseIsolation) error {
	desired, err := rt.databases.GetDesiredDatabase(ctx, p.DatabaseName)
	if err != nil && !errors.Is(err, store.ErrDatabaseNotFound) {
		return fmt.Errorf("load database: %w", err)
	}
	if err == nil && rt.execRuntime != nil {
		if execErr := rt.execIsolatedRoleSQL(ctx, p.DatabaseName, desired.NodeID, database.DropIsolatedRoleSQL(p.RoleName, p.DatabaseName)); execErr != nil {
			return fmt.Errorf("drop role: %w", execErr)
		}
	}

	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(ctx, previewDatabaseIsolationSecretService(p.PreviewEnvironmentID, p.SourceKey)); err != nil {
			rt.logger.Warn("api: teardown preview database isolation: delete secret failed", slog.String("error", err.Error()), slog.String("role", p.RoleName))
		}
	}

	if err := rt.previewEnvironments.DeletePreviewDatabaseIsolation(ctx, p.ID); err != nil && !errors.Is(err, store.ErrPreviewDatabaseIsolationNotFound) {
		return fmt.Errorf("delete tracking row: %w", err)
	}
	return nil
}

// previewDatabaseIsolationSecretService is this isolation's own
// dedicated secrets namespace: never the preview app's own service
// name, so DeleteAll here can never remove an unrelated app secret.
func previewDatabaseIsolationSecretService(previewEnvironmentID, sourceKey string) string {
	return "preview-db-isolation:" + previewEnvironmentID + ":" + sourceKey
}

// execIsolatedRoleSQL runs sql against dbName's running container as a
// single psql -c call using the container's own trusted local
// connection (no password needed, the same "$POSTGRES_USER" convention
// internal/backup/pitr_runner.go already uses for its own read-only
// exec), so no admin credential needs resolving here.
func (rt *Router) execIsolatedRoleSQL(ctx context.Context, dbName, nodeID, sql string) error {
	runtime, err := rt.execRuntime(nodeID)
	if err != nil {
		return fmt.Errorf("resolve node runtime: %w", err)
	}

	inspectCtx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	state, err := runtime.InspectByName(inspectCtx, databaseContainerName(dbName))
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}
	if state == nil || !state.Running {
		return fmt.Errorf("database %q has no running container", dbName)
	}

	cmd := []string{"sh", "-c", `exec psql --no-password -U "$POSTGRES_USER" -v ON_ERROR_STOP=1 -c ` + shellSingleQuote(sql)}
	execCtx, execCancel := context.WithTimeout(ctx, isolatedRoleExecTimeout)
	defer execCancel()
	rc, err := runtime.Exec(execCtx, state.ID, cmd)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	defer func() { _ = rc.Close() }()

	capped := &cappedWriter{limit: execMaxOutputBytes}
	if _, err := io.Copy(capped, rc); err != nil {
		var execErr *docker.ExecExitError
		if errors.As(err, &execErr) {
			return fmt.Errorf("psql exited %d: %s", execErr.ExitCode, execErr.Stderr)
		}
		return fmt.Errorf("read exec output: %w", err)
	}
	return nil
}

// shellSingleQuote wraps s in single quotes for a "sh -c" argument,
// escaping any embedded single quote. sql only ever comes from
// database.CreateIsolatedRoleSQL/DropIsolatedRoleSQL in this file.
func shellSingleQuote(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
			continue
		}
		out += string(r)
	}
	return out + "'"
}
