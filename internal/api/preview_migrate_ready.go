package api

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewDatabaseProbe is the in-container readiness command per engine. It
// probes over TCP because the image init server only listens on the socket.
// An engine without an entry cannot be verified, so a migration is refused.
func previewDatabaseProbe(engine string) []string {
	if engine == spec.EnginePostgres {
		return []string{"pg_isready", "-q", "-h", "127.0.0.1"}
	}
	return nil
}

// waitPreviewDatabaseReady polls the preview database container until its
// engine accepts connections, bounded by wait. It fails closed on timeout.
func waitPreviewDatabaseReady(ctx context.Context, rtm docker.Runtime, container, engine string, wait time.Duration) error {
	probe := previewDatabaseProbe(engine)
	if probe == nil {
		return errors.New("the " + engine + " preview database cannot be verified ready before the migration")
	}
	deadline := time.Now().Add(wait)
	for {
		if previewDatabaseAccepts(ctx, rtm, container, probe) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return errors.New("the preview database was not ready in time")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(previewMigratePoll):
		}
	}
}

func previewDatabaseAccepts(ctx context.Context, rtm docker.Runtime, container string, probe []string) bool {
	inspectCtx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	state, err := rtm.InspectByName(inspectCtx, container)
	cancel()
	if err != nil || state == nil || !state.Running {
		return false
	}
	rc, err := rtm.Exec(ctx, state.ID, probe)
	if err != nil {
		return false
	}
	defer func() { _ = rc.Close() }()
	// A non-zero exit surfaces as an error once the output is drained.
	_, err = io.Copy(io.Discard, rc)
	return err == nil
}

// waitFreshPreviewDatabase runs the readiness wait for a database this
// preview just created, on the node that hosts it.
func (rt *Router) waitFreshPreviewDatabase(ctx context.Context, s store.PreviewAppSettings, db store.PreviewEphemeralDatabase, engine string) error {
	if rt.execRuntime == nil {
		return errors.New("migration skipped: this control plane cannot check the preview database")
	}
	nodeID := ""
	if d, err := rt.databases.GetDesiredDatabase(ctx, db.DatabaseName); err == nil {
		nodeID = d.NodeID
	}
	rtm, err := rt.execRuntime(nodeID)
	if err != nil {
		return errors.New("migration skipped: the preview database node is not reachable")
	}
	bound := previewMigrateTimeoutOr(s, rt.previewMigratePlatformTimeout(ctx))
	waitCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	if err := waitPreviewDatabaseReady(waitCtx, rtm, databaseContainerName(db.DatabaseName), engine, bound); err != nil {
		return errors.New("migration skipped: " + err.Error())
	}
	return nil
}
