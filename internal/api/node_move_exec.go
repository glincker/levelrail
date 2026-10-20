package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/nodemove"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// moveExecutor implements nodemove.Executor and nodemove.DrainPlanner
// against the router's stores and node runtimes.
type moveExecutor struct{ rt *Router }

var (
	_ nodemove.Executor     = (*moveExecutor)(nil)
	_ nodemove.DrainPlanner = (*moveExecutor)(nil)
)

func (e *moveExecutor) sameNode(a, b string) bool {
	return application.SameNode(a, b, e.rt.localNodeID)
}

func (e *moveExecutor) meshEnabled() bool { return e.rt.localNodeID != "" }

// Preflight refuses moves that cannot work and prepares database mesh
// endpoints for any connection the move makes cross-node.
func (e *moveExecutor) Preflight(ctx context.Context, m store.NodeMove) error {
	current, err := e.rt.currentNode(ctx, m.Kind, m.Resource)
	if isResourceNotFound(err) {
		return nodemove.Permanent(fmt.Errorf("%s %q no longer exists", m.Kind, m.Resource))
	} else if err != nil {
		return err
	}
	if !e.sameNode(current, m.FromNodeID) {
		return nodemove.Permanent(fmt.Errorf("%s %q was placed elsewhere since this move started", m.Kind, m.Resource))
	}
	if err := e.rt.checkPlacement(ctx, m.Kind, m.Resource, m.ToNodeID); err != nil {
		return nodemove.Permanent(err)
	}
	inDrain := m.DrainID != ""
	if m.Kind == store.NodeMoveKindApp {
		svc, err := e.rt.apps.GetDesiredService(ctx, m.Resource)
		if err != nil {
			return err
		}
		if len(svc.BindMounts) > 0 {
			return nodemove.Permanent(fmt.Errorf("app %q mounts host directories, which cannot be copied between nodes: replace them with named volumes or move the data by hand", m.Resource))
		}
		for _, dbName := range serviceDatabaseRefs(svc) {
			db, err := e.rt.databases.GetDesiredDatabase(ctx, dbName)
			if errors.Is(err, store.ErrDatabaseNotFound) {
				continue
			} else if err != nil {
				return err
			}
			if err := e.prepareCrossNode(ctx, db.Name, !e.sameNode(db.NodeID, m.ToNodeID), inDrain); err != nil {
				return err
			}
		}
	} else {
		dependents, err := e.dependents(ctx, m.Resource)
		if err != nil {
			return err
		}
		cross := false
		for _, svc := range dependents {
			cross = cross || !e.sameNode(svc.NodeID, m.ToNodeID)
		}
		if err := e.prepareCrossNode(ctx, m.Resource, cross, inDrain); err != nil {
			return err
		}
	}
	if m.Stateful {
		if _, err := e.rt.execRuntime(m.FromNodeID); err != nil {
			return fmt.Errorf("source node is not reachable, its data cannot be copied yet: %w", err)
		}
	}
	return nil
}

// prepareCrossNode allocates dbName's mesh port when a connection to it
// becomes cross-node. Without the mesh such a move is refused, except
// inside a drain, where the app follows its database onto the same node.
func (e *moveExecutor) prepareCrossNode(ctx context.Context, dbName string, cross, inDrain bool) error {
	if !cross {
		return nil
	}
	if !e.meshEnabled() {
		if inDrain {
			return nil
		}
		return nodemove.Permanent(fmt.Errorf("this move puts database %q and an app that uses it on different nodes, which needs mesh networking (APP_MESH_ENABLED=1); move them together instead", dbName))
	}
	if _, err := e.rt.multiNode.store.EnsureDatabaseMeshPort(ctx, dbName, e.rt.multiNode.portMin, e.rt.multiNode.portMax); err != nil {
		return nodemove.Permanent(fmt.Errorf("allocate a mesh port for database %q: %w", dbName, err))
	}
	return nil
}

func serviceDatabaseRefs(svc *store.DesiredService) []string {
	seen := map[string]bool{}
	var out []string
	for _, ref := range svc.DatabaseEnv {
		if !seen[ref.Database] {
			seen[ref.Database] = true
			out = append(out, ref.Database)
		}
	}
	if att := svc.DatabaseAttachment; att != nil && !seen[att.DatabaseName] {
		out = append(out, att.DatabaseName)
	}
	sort.Strings(out)
	return out
}

func (e *moveExecutor) dependents(ctx context.Context, dbName string) ([]store.DesiredService, error) {
	all, err := e.rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.DesiredService
	for i := range all {
		for _, ref := range serviceDatabaseRefs(&all[i]) {
			if ref == dbName {
				out = append(out, all[i])
				break
			}
		}
	}
	return out, nil
}

func (e *moveExecutor) Stop(ctx context.Context, kind, name, nodeID string) error {
	runtime, err := e.rt.execRuntime(nodeID)
	if err != nil {
		return fmt.Errorf("resolve node runtime: %w", err)
	}
	if kind == store.NodeMoveKindDatabase {
		if err := e.rt.databases.UpdateDatabaseSuspended(ctx, name, true); err != nil {
			return err
		}
		return database.New(name, e.rt.databases, runtime).Teardown(ctx)
	}
	if err := e.rt.apps.UpdateServiceSuspended(ctx, name, true); err != nil {
		return err
	}
	return application.New(name, e.rt.apps, runtime).Teardown(ctx)
}

func (e *moveExecutor) CopyVolume(ctx context.Context, fromNodeID, toNodeID, volume string) error {
	src, err := e.rt.execRuntime(fromNodeID)
	if err != nil {
		return fmt.Errorf("resolve source runtime: %w", err)
	}
	dst, err := e.rt.execRuntime(toNodeID)
	if err != nil {
		return fmt.Errorf("resolve target runtime: %w", err)
	}
	return backup.MoveVolume(ctx, &backup.ContainerVolumeArchiver{Runtime: src}, &backup.ContainerVolumeRestorer{Runtime: dst}, volume)
}

func (e *moveExecutor) Place(ctx context.Context, kind, name, nodeID string) error {
	var err error
	if kind == store.NodeMoveKindDatabase {
		err = e.rt.databases.UpdateDatabaseNode(ctx, name, nodeID)
	} else {
		err = e.rt.apps.UpdateServiceNode(ctx, name, nodeID)
	}
	e.rt.nudgeReconciler()
	return err
}

func (e *moveExecutor) Resume(ctx context.Context, kind, name string) error {
	var err error
	if kind == store.NodeMoveKindDatabase {
		err = e.rt.databases.UpdateDatabaseSuspended(ctx, name, false)
	} else {
		err = e.rt.apps.UpdateServiceSuspended(ctx, name, false)
	}
	e.rt.nudgeReconciler()
	return err
}

// Ready needs both a fresh Ready=True condition (reported after since)
// and a running container on nodeID, so a stale report from the source
// node can never pass for the target.
func (e *moveExecutor) Ready(ctx context.Context, kind, name, nodeID string, since time.Time) (bool, string, error) {
	controller := applicationControllerName(name)
	if kind == store.NodeMoveKindDatabase {
		controller = databaseControllerName(name)
	}
	conds, err := e.rt.deploys.GetConditions(ctx, controller)
	if err != nil {
		return false, "", err
	}
	// reconcile_status stores milliseconds.
	since = since.Truncate(time.Millisecond)
	reason := "waiting for the first report from the target node"
	fresh := false
	for _, c := range conds {
		if c.Type != reconcile.ConditionTypeReady || c.LastTransitionTime.Before(since) {
			continue
		}
		fresh = c.Status == reconcile.ConditionTrue
		if !fresh {
			reason = strings.TrimSpace(c.Reason + ": " + c.Message)
		}
	}
	if !fresh {
		return false, reason, nil
	}
	runtime, err := e.rt.execRuntime(nodeID)
	if err != nil {
		return false, "", fmt.Errorf("resolve target runtime: %w", err)
	}
	if kind == store.NodeMoveKindDatabase {
		st, err := runtime.InspectByName(ctx, database.ContainerName(name))
		if err != nil {
			return false, "", err
		}
		if st == nil || !st.Running {
			return false, "database container is not running on the target node", nil
		}
		return true, "", nil
	}
	containers, err := runtime.ListByPrefix(ctx, name+"-")
	if err != nil {
		return false, "", err
	}
	for _, c := range containers {
		if svc, ok := application.ParseContainerName(c.Name); ok && svc == name && c.Running {
			return true, "", nil
		}
	}
	return false, "no running container on the target node yet", nil
}

// RemoveFrom tears down an app's containers on one node. When that node
// is unreachable the teardown is queued with the delete finalizer, which
// retries, so a dead source never blocks a verified move.
func (e *moveExecutor) RemoveFrom(ctx context.Context, kind, name, nodeID string) error {
	runtime, err := e.rt.execRuntime(nodeID)
	if err == nil {
		if kind == store.NodeMoveKindDatabase {
			err = database.New(name, e.rt.databases, runtime).Teardown(ctx)
		} else {
			err = application.New(name, e.rt.apps, runtime).Teardown(ctx)
		}
	}
	if err == nil || kind == store.NodeMoveKindDatabase {
		return err
	}
	e.rt.logger.Warn("nodemove: direct teardown failed, queued for retry", slog.String("app", name), slog.String("node_id", nodeID), slog.String("error", err.Error()))
	e.rt.teardownServiceContainers(name, nodeID)
	return nil
}

// Finalize restarts a moved database's dependents so they pick up its new
// address; their connection name and port stay the same.
func (e *moveExecutor) Finalize(ctx context.Context, m store.NodeMove) error {
	if m.Kind != store.NodeMoveKindDatabase {
		return nil
	}
	dependents, err := e.dependents(ctx, m.Resource)
	if err != nil {
		return err
	}
	var failed []string
	for _, svc := range dependents {
		if err := e.rt.apps.RestartService(ctx, svc.Name); err != nil {
			failed = append(failed, svc.Name)
		}
	}
	e.rt.nudgeReconciler()
	if len(failed) > 0 {
		return fmt.Errorf("restart dependents %s", strings.Join(failed, ", "))
	}
	return nil
}

func (e *moveExecutor) Cordon(ctx context.Context, nodeID string) error {
	return e.rt.nodes.SetNodeSchedulable(ctx, nodeID, false)
}

// Remaining lists databases first, then apps, so apps can follow.
func (e *moveExecutor) Remaining(ctx context.Context, nodeID string) ([]nodemove.Resource, error) {
	ids := []string{nodeID}
	if nodeID == e.rt.localNodeID && nodeID != "" {
		ids = append(ids, "")
	}
	var out []nodemove.Resource
	for _, id := range ids {
		dbs, err := e.rt.databases.ListDesiredDatabasesByNode(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, d := range dbs {
			out = append(out, nodemove.Resource{Kind: store.NodeMoveKindDatabase, Name: d.Name, Volumes: databaseVolumes(d)})
		}
	}
	for _, id := range ids {
		svcs, err := e.rt.apps.ListDesiredServicesByNode(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, s := range svcs {
			out = append(out, nodemove.Resource{Kind: store.NodeMoveKindApp, Name: s.Name, Volumes: serviceVolumes(s)})
		}
	}
	return out, nil
}

func databaseVolumes(d store.DesiredDatabase) []string {
	vols := []string{database.DataVolumeName(d.Name)}
	if d.PITREnabled {
		vols = append(vols, database.WALArchiveVolumeName(d.Name))
	}
	return vols
}

func serviceVolumes(s store.DesiredService) []string {
	out := make([]string, 0, len(s.Volumes))
	for _, v := range s.Volumes {
		out = append(out, v.Name)
	}
	return out
}

// PickTarget honors an explicit drain target, keeps an app with its
// database when the database's node is eligible, sends GPU apps through
// the GPU planner, and otherwise spreads.
func (e *moveExecutor) PickTarget(ctx context.Context, res nodemove.Resource, fromNodeID, explicit string) (string, error) {
	if res.Kind == store.NodeMoveKindApp {
		svc, err := e.rt.apps.GetDesiredService(ctx, res.Name)
		if err != nil {
			return "", err
		}
		if claim, ok := models.ServiceClaim(*svc); ok && e.rt.models != nil {
			return e.pickGPU(ctx, fromNodeID, explicit, claim)
		}
		if explicit == "" {
			if node, ok := e.followDatabase(ctx, svc, fromNodeID); ok {
				return node, nil
			}
		}
	}
	if explicit != "" {
		if err := e.rt.checkPlacement(ctx, res.Kind, res.Name, explicit); err != nil {
			return "", &nodemove.BlockedError{Reason: err.Error()}
		}
		return explicit, nil
	}
	target, err := e.rt.spreadTarget(ctx, res.Kind, res.Name, fromNodeID, placementTriggerDrain)
	if err != nil {
		return "", &nodemove.BlockedError{Reason: err.Error()}
	}
	return target, nil
}

func (e *moveExecutor) followDatabase(ctx context.Context, svc *store.DesiredService, fromNodeID string) (string, bool) {
	for _, dbName := range serviceDatabaseRefs(svc) {
		db, err := e.rt.databases.GetDesiredDatabase(ctx, dbName)
		if err != nil || e.sameNode(db.NodeID, fromNodeID) {
			continue
		}
		if e.rt.checkPlacement(ctx, store.NodeMoveKindApp, svc.Name, db.NodeID) == nil {
			return db.NodeID, true
		}
	}
	return "", false
}

func (e *moveExecutor) pickGPU(ctx context.Context, fromNodeID, explicit string, claim gpu.Claim) (string, error) {
	placer := &drainPlacer{rt: e.rt, fromID: fromNodeID, explicit: explicit != "", target: explicit}
	target, err := placer.next(ctx, &claim, drainRef{})
	var noGPU *errNoGPUTarget
	if errors.As(err, &noGPU) {
		return "", &nodemove.BlockedError{Reason: noGPU.reason}
	}
	return target, err
}

func (e *moveExecutor) Blocked(ctx context.Context, nodeID string) ([]store.NodeDrainBlocked, error) {
	blocks, err := e.rt.drainModelBlocks(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	out := make([]store.NodeDrainBlocked, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, store.NodeDrainBlocked{Kind: b.Kind, Name: b.Name, Reason: b.Reason})
	}
	return out, nil
}
