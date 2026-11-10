package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbparams"
	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// paramsRollbacker is what the SQLite store offers for automatic rollback;
// other stores simply never roll back.
type paramsRollbacker interface {
	GetDatabaseEngineParams(ctx context.Context, name string) (store.DatabaseEngineParams, error)
	RollbackDatabaseEngineParams(ctx context.Context, name string, expected map[string]string, expectedAppliedAt, reason string, at time.Time, audit store.AuditEntry) (bool, error)
}

// paramsWatch is the first start of a new tuning set after a running
// container with other args: started is when the new container started.
type paramsWatch struct {
	label    string
	started  time.Time
	vanished int
}

// watchParamsStart begins a watch when the args changed on a container that
// was running the same image before; a fresh create, an image change or a
// remote node (no probe can reach it) has no baseline to blame the args against.
func (c *Controller) watchParamsStart(baseline bool, prevLabel, newLabel string) {
	if !baseline || c.remoteNode || prevLabel == newLabel {
		return
	}
	c.shared.setWatch(c.dbName, paramsWatch{label: newLabel, started: c.clock()})
}

func (c *Controller) noteVanished(label string) { c.shared.noteVanished(c.dbName, label) }

// watchFor returns the watch when it still follows state's args.
func (c *Controller) watchFor(state *docker.ContainerState) (paramsWatch, bool) {
	w, ok := c.shared.watch(c.dbName)
	if !ok || state == nil || state.Labels[engineParamsLabelKey] != w.label {
		return paramsWatch{}, false
	}
	return w, true
}

// minVanishedStarts is how many starts of the new args must exit at once
// before that counts as their fault rather than a one-off host problem.
const minVanishedStarts = 2

// watchVerdict says whether reason settles the watch, and whether it blames
// the new args. A single exit right after start is not yet a verdict.
func watchVerdict(reason string, w paramsWatch, exitCode int, exitKnown bool) (settled, blame bool) {
	switch reason {
	case reasonNotAccepting, dbprovision.ReasonCrashLoop:
		return true, true
	case "VanishedAfterStart":
		if w.vanished < minVanishedStarts {
			return false, false
		}
		return true, !exitKnown || exitCode != 0
	}
	return false, false
}

// paramsRollbackEligible checks the stored sets: the failing args are the
// applied set, the previous set differs, and no rollback happened since the apply.
func paramsRollbackEligible(engine string, p store.DatabaseEngineParams, label string) bool {
	if p.RolledBackAt != "" || p.AppliedAt == "" {
		return false
	}
	cur := dbparams.Fingerprint(dbparams.Args(engine, p.Applied, dbparams.Limits{}))
	prev := dbparams.Fingerprint(dbparams.Args(engine, p.Previous, dbparams.Limits{}))
	return cur != prev && label == cur
}

// rollbackFailedParams restores the previous set once when the watched start
// of a new set failed; the next pass replaces the container with it.
func (c *Controller) rollbackFailedParams(ctx context.Context, res reconcile.Result) reconcile.Result {
	w, watching := c.shared.watch(c.dbName)
	rb, ok := c.store.(paramsRollbacker)
	if !watching || !ok || len(res.Conditions) == 0 {
		return res
	}
	reason := res.Conditions[0].Reason
	exitCode, exitKnown := c.exitCodeOf(ctx)
	settled, blame := watchVerdict(reason, w, exitCode, exitKnown)
	if !settled {
		return res
	}
	c.shared.clearWatch(c.dbName)
	if !blame {
		return res
	}
	desired, err := c.store.GetDesiredDatabase(ctx, c.dbName)
	if err != nil || desired.Suspended {
		return res
	}
	p, err := rb.GetDatabaseEngineParams(ctx, c.dbName)
	// The label fingerprints the stored set without limits, see engineParamsSpec.
	if err != nil || !paramsRollbackEligible(desired.Engine, p, w.label) {
		return res
	}
	now := c.clock()
	rolled, err := rb.RollbackDatabaseEngineParams(ctx, c.dbName, p.Applied, p.AppliedAt, reason, now, c.rollbackAudit(p, reason, now))
	if err != nil || !rolled {
		return res
	}
	cond := &res.Conditions[0]
	cond.Message += " (the previous engine parameter set was restored automatically and the container is replaced with it)"
	res.RequeueAfter = durationFromEnv(EnvReadyProbeInterval, defaultReadyProbe)
	return res
}

// exitCodeOf reads the last exit code when the runtime reports it.
func (c *Controller) exitCodeOf(ctx context.Context) (int, bool) {
	insp, ok := c.runtime.(docker.ExitStateInspector)
	if !ok {
		return 0, false
	}
	st, err := insp.InspectExitState(ctx, containerName(c.dbName))
	if err != nil || st == nil {
		return 0, false
	}
	return st.ExitCode, true
}

func (c *Controller) rollbackAudit(p store.DatabaseEngineParams, reason string, now time.Time) store.AuditEntry {
	id, err := store.NewAuditEntryID()
	if err != nil {
		id = "aud_rollback_" + now.UTC().Format("20060102T150405.000000000")
	}
	// Every allowlisted parameter is a non-secret tuning number, so values are kept.
	detail, _ := json.Marshal(map[string]any{"database": c.dbName, "reason": reason, "from": p.Applied, "to": p.Previous})
	return store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "database-reconciler", ActorName: "database reconciler",
		Ability: "system", Path: "/api/v1/databases/" + c.dbName + "/params", StatusCode: 200,
		CreatedAt: store.FormatAuditTime(now), ClientKind: "system",
		Action: store.AuditActionDatabaseParamsAutoRollback, Detail: string(detail),
	}
}
