package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const databaseTeardownTimeout = 2 * time.Minute

// databaseTeardownJobStore persists teardowns that outlive the request that asked for them.
type databaseTeardownJobStore interface {
	SaveDatabaseTeardownJob(ctx context.Context, name, nodeID string, purge bool) error
	ListDatabaseTeardownJobs(ctx context.Context) ([]store.DatabaseTeardownJob, error)
	FinishDatabaseTeardownJob(ctx context.Context, name, nodeID string) error
	FailDatabaseTeardownJob(ctx context.Context, name, nodeID, reason string) error
}

func (rt *Router) teardownJobStore() databaseTeardownJobStore {
	js, _ := rt.databases.(databaseTeardownJobStore)
	return js
}

func (rt *Router) recordDatabaseTeardown(ctx context.Context, name, nodeID string, purge bool) error {
	js := rt.teardownJobStore()
	if js == nil {
		return nil
	}
	if err := js.SaveDatabaseTeardownJob(ctx, name, nodeID, purge); err != nil {
		return fmt.Errorf("record database teardown: %w", err)
	}
	return nil
}

func (rt *Router) scheduleDatabaseTeardown(name, nodeID string, purge bool) {
	if rt.execRuntime == nil {
		return
	}
	if err := rt.recordDatabaseTeardown(context.Background(), name, nodeID, purge); err != nil {
		rt.logger.Error("api: database teardown not recorded, running it once without retry", slog.String("name", name), slog.String("error", err.Error()))
	}
	go func() { //nolint:gosec // deliberately outlives the request, bounded by databaseTeardownTimeout and retried from the job row
		if err := rt.runDatabaseTeardown(context.Background(), name, nodeID, purge); err != nil {
			rt.logger.Error("api: database teardown failed, will retry", slog.String("name", name), slog.String("error", err.Error()))
		}
	}()
}

// runDatabaseTeardown stops and removes name's container on nodeID (and its volumes when purge), then clears the job. A failure leaves the job for the next retry.
func (rt *Router) runDatabaseTeardown(ctx context.Context, name, nodeID string, purge bool) error {
	if rt.execRuntime == nil {
		return nil
	}
	tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), databaseTeardownTimeout)
	defer cancel()

	err := rt.teardownOnce(tctx, name, nodeID, purge)
	js := rt.teardownJobStore()
	if js == nil {
		return err
	}
	if err != nil {
		if ferr := js.FailDatabaseTeardownJob(tctx, name, nodeID, err.Error()); ferr != nil {
			rt.logger.Error("api: record database teardown failure failed", slog.String("name", name), slog.String("error", ferr.Error()))
		}
		return err
	}
	if ferr := js.FinishDatabaseTeardownJob(tctx, name, nodeID); ferr != nil {
		rt.logger.Error("api: clear database teardown job failed", slog.String("name", name), slog.String("error", ferr.Error()))
	}
	return nil
}

func (rt *Router) teardownOnce(ctx context.Context, name, nodeID string, purge bool) error {
	runtime, err := rt.execRuntime(nodeID)
	if err != nil {
		return fmt.Errorf("resolve node runtime: %w", err)
	}
	if err := database.New(name, rt.databases, runtime).Teardown(ctx); err != nil {
		return err
	}
	if purge {
		names := databaseVolumeNames(name)
		if !rt.isLocalNode(nodeID) {
			return removeRemoteVolumes(ctx, runtime, names)
		}
		rt.purgeVolumes(ctx, "database/"+name, names)
	}
	return nil
}

// RetryDatabaseTeardowns reruns every pending teardown. Safe to call repeatedly: a finished job is gone, a failed one stays.
func (rt *Router) RetryDatabaseTeardowns(ctx context.Context) error {
	js := rt.teardownJobStore()
	if js == nil || rt.execRuntime == nil {
		return nil
	}
	jobs, err := js.ListDatabaseTeardownJobs(ctx)
	if err != nil {
		return fmt.Errorf("list database teardown jobs: %w", err)
	}
	var errs []error
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		if rt.databaseTeardownSuperseded(ctx, j, js) {
			continue
		}
		if err := rt.runDatabaseTeardown(ctx, j.Name, j.NodeID, j.Purge); err != nil {
			errs = append(errs, fmt.Errorf("database %q: %w", j.Name, err))
		}
	}
	return errors.Join(errs...)
}

// databaseTeardownSuperseded drops a job whose database name is live again on
// the same node, so a retry never tears down or purges the new database. A
// failed lookup also skips the job: the next retry decides.
func (rt *Router) databaseTeardownSuperseded(ctx context.Context, j store.DatabaseTeardownJob, js databaseTeardownJobStore) bool {
	d, err := rt.databases.GetDesiredDatabase(ctx, j.Name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		return false
	}
	if err != nil {
		rt.logger.Error("api: database teardown retry skipped, lookup failed", slog.String("name", j.Name), slog.String("error", err.Error()))
		return true
	}
	if d.NodeID != j.NodeID {
		return false
	}
	rt.logger.Warn("api: stale database teardown dropped, the name is live again", slog.String("name", j.Name), slog.String("node_id", j.NodeID))
	if ferr := js.FinishDatabaseTeardownJob(ctx, j.Name, j.NodeID); ferr != nil {
		rt.logger.Error("api: clear superseded teardown job failed", slog.String("name", j.Name), slog.String("error", ferr.Error()))
	}
	return true
}

// databaseTeardownPending reports whether a teardown for name on nodeID is still owed.
func (rt *Router) databaseTeardownPending(ctx context.Context, name, nodeID string) (bool, error) {
	js := rt.teardownJobStore()
	if js == nil {
		return false, nil
	}
	jobs, err := js.ListDatabaseTeardownJobs(ctx)
	if err != nil {
		return false, fmt.Errorf("list database teardown jobs: %w", err)
	}
	for _, j := range jobs {
		if j.Name == name && j.NodeID == nodeID {
			return true, nil
		}
	}
	return false, nil
}

// removeRemoteVolumes deletes volumes on a remote node through its agent. A failure
// is returned so the teardown job retries; a volume that is already gone is fine.
func removeRemoteVolumes(ctx context.Context, rt docker.Runtime, names []string) error {
	remover, ok := rt.(docker.VolumeRemover)
	if !ok {
		return errors.New("this node's agent cannot remove volumes; upgrade the agent or remove them on the node")
	}
	var errs []error
	for _, n := range names {
		if err := remover.RemoveVolume(ctx, n); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such volume") {
			errs = append(errs, fmt.Errorf("remove volume %q: %w", n, err))
		}
	}
	return errors.Join(errs...)
}
