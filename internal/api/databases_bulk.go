package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	dbBulkStart  = "start"
	dbBulkStop   = "stop"
	dbBulkBackup = "backup"

	dbBulkPlanned   = "planned"
	dbBulkProtected = "protected"

	dbSkipRunning   = "running"
	dbSkipStopped   = "stopped"
	dbSkipNoTarget  = "noTarget"
	dbSkipNoBackups = "noBackups"
)

// dbBulkAbility is the ability each database must grant: the same one the
// single-database start, stop and backup routes require, never weaker.
var dbBulkAbility = map[string]string{
	dbBulkStart:  AbilityWriteSensitive,
	dbBulkStop:   AbilityWriteSensitive,
	dbBulkBackup: AbilityWriteSensitive,
}

type bulkDatabasesRequest struct {
	Action string `json:"action"`
	// Names lists targets; the selector fields apply only when Names is empty.
	Names       []string `json:"names,omitempty"`
	Engine      string   `json:"engine,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	Environment string   `json:"environment_id,omitempty"`
	DryRun      bool     `json:"dry_run,omitempty"`
	// ConfirmCount must equal the resolved target count for a real stop.
	ConfirmCount int `json:"confirm_count,omitempty"`
}

type bulkDatabaseResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	// Reason is a stable code for a skip (running, stopped, noTarget, noBackups).
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// BackupID is the backup history row a started backup reports into.
	BackupID string `json:"backup_id,omitempty"`
}

type bulkDatabasesResponse struct {
	Action  string               `json:"action"`
	DryRun  bool                 `json:"dry_run"`
	Results []bulkDatabaseResult `json:"results"`
	Counts  map[string]int       `json:"counts"`
}

// visibleDatabase is false for a database a scoped caller cannot see; it
// then answers exactly like a missing one, as the single routes do.
func (p bulkPrincipal) visibleDatabase(name string) bool {
	return p.scope == nil || (p.scope.visible(resourcePrefixDatabase, name) && p.scope.authorize(p.abilities, p.policies, AbilityRead, resourcePrefixDatabase, name))
}

func (p bulkPrincipal) allowedDatabase(ability, name string) bool {
	if p.scope != nil {
		return p.scope.authorize(p.abilities, p.policies, ability, resourcePrefixDatabase, name)
	}
	return authorizeResource(p.abilities, p.policies, ability, resourcePrefixDatabase+name)
}

type dbBackupJob struct{ historyID, name, engine, target string }

// handleBulkDatabases handles POST /api/v1/databases/bulk. Every database is
// authorized and audited on its own, so one denial or failure never hides
// or blocks the rest. Answers 207 once the request itself is valid.
func (rt *Router) handleBulkDatabases(w http.ResponseWriter, r *http.Request) {
	var req bulkDatabasesRequest
	if !decodeReq(w, r, &req) {
		return
	}
	ability, ok := dbBulkAbility[req.Action]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown action: use start, stop or backup")
		return
	}
	// A disconnect must not drop audit rows for changes that already landed.
	ctx := context.WithoutCancel(r.Context())
	maxN := rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDatabaseBulkMaxItems)
	if len(req.Names) > maxN {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many databases (%d, max %d)", len(req.Names), maxN))
		return
	}
	for _, n := range req.Names {
		if !validDatabaseName(n) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid database name %q", n))
			return
		}
	}
	principal, err := rt.bulkPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: bulk databases: resolve caller failed", err)
		return
	}
	names, err := rt.resolveBulkDatabases(ctx, req, principal)
	if err != nil {
		rt.internalError(w, "api: bulk databases: resolve targets failed", err)
		return
	}
	if len(names) == 0 {
		writeError(w, http.StatusBadRequest, "no target databases: set names or a selector that matches")
		return
	}
	if len(names) > maxN {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many databases (%d, max %d)", len(names), maxN))
		return
	}
	if req.Action == dbBulkStop && !req.DryRun && req.ConfirmCount != len(names) {
		writeError(w, http.StatusConflict, fmt.Sprintf("stop of %d databases requires confirm_count: %d", len(names), len(names)))
		return
	}

	limit := rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDatabaseBulkConcurrency)
	results := make([]bulkDatabaseResult, len(names))
	var (
		jobsMu sync.Mutex
		jobs   []dbBackupJob
	)
	enqueue := func(j dbBackupJob) {
		jobsMu.Lock()
		jobs = append(jobs, j)
		jobsMu.Unlock()
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = rt.applyBulkDatabase(ctx, principal, ability, req, name, enqueue)
			if !req.DryRun {
				rt.recordDatabaseBulkAudit(ctx, r, principal, ability, req.Action, results[i])
			}
		}()
	}
	wg.Wait()
	rt.runBulkBackups(jobs, limit)

	counts := map[string]int{}
	for _, res := range results {
		counts[res.Status]++
	}
	if !req.DryRun && req.Action != dbBulkBackup && counts[bulkStatusOK] > 0 {
		rt.nudgeReconciler()
	}
	writeJSON(w, http.StatusMultiStatus, bulkDatabasesResponse{Action: req.Action, DryRun: req.DryRun, Results: results, Counts: counts})
}

// runBulkBackups runs queued dumps in the background, at most limit at once,
// so one bulk call cannot start every dump together and fill the disk.
func (rt *Router) runBulkBackups(jobs []dbBackupJob, limit int) {
	if len(jobs) == 0 {
		return
	}
	go func() {
		sem := make(chan struct{}, limit)
		var wg sync.WaitGroup
		for _, j := range jobs {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if err := rt.backupRunner.RunBackup(context.Background(), j.historyID, j.name, j.engine, databaseContainerName(j.name), j.target); err != nil {
					rt.logger.Error("api: bulk backup run failed", slog.String("error", err.Error()), slog.String("id", j.historyID), slog.String("database", j.name))
				}
			}()
		}
		wg.Wait()
	}()
}

// resolveBulkDatabases keeps explicit names (hidden ones answer not_found per
// item) but drops hidden databases from a selector, so counts never leak.
func (rt *Router) resolveBulkDatabases(ctx context.Context, req bulkDatabasesRequest, p bulkPrincipal) ([]string, error) {
	if len(req.Names) > 0 {
		names := slices.Clone(req.Names)
		slices.Sort(names)
		return slices.Compact(names), nil
	}
	if req.Engine == "" && req.ProjectID == "" && req.Environment == "" {
		return nil, nil
	}
	var inEnv map[string]bool
	if req.Environment != "" {
		es, ok := rt.databases.(databaseEnvironmentStore)
		if !ok {
			return nil, errNoDatabaseEnvironmentStore
		}
		m, err := es.DatabaseNamesInEnvironment(ctx, req.Environment)
		if err != nil {
			return nil, fmt.Errorf("list databases in environment: %w", err)
		}
		inEnv = m
	}
	all, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	var out []string
	for _, d := range all {
		if (req.Engine != "" && d.Engine != req.Engine) || (req.ProjectID != "" && d.ProjectID != req.ProjectID) || (inEnv != nil && !inEnv[d.Name]) {
			continue
		}
		if !p.visibleDatabase(d.Name) {
			continue
		}
		out = append(out, d.Name)
	}
	slices.Sort(out)
	return out, nil
}

// applyBulkDatabase answers hidden and missing databases alike, and checks
// authz before existence, so no caller can probe which names exist.
func (rt *Router) applyBulkDatabase(ctx context.Context, p bulkPrincipal, ability string, req bulkDatabasesRequest, name string, enqueue func(dbBackupJob)) bulkDatabaseResult {
	res := bulkDatabaseResult{Name: name}
	fail := func(what string, err error) bulkDatabaseResult {
		rt.logger.Error("api: bulk databases: "+what+" failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("action", req.Action))
		res.Status, res.Message = bulkStatusError, what+" failed"
		return res
	}
	if !p.visibleDatabase(name) {
		res.Status, res.Message = bulkStatusNotFound, "database not found"
		return res
	}
	if !p.allowedDatabase(ability, name) {
		res.Status, res.Message = bulkStatusDenied, "your account lacks the required ability on this database"
		return res
	}
	db, err := rt.databases.GetDesiredDatabase(ctx, name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		res.Status, res.Message = bulkStatusNotFound, "database not found"
		return res
	}
	if err != nil {
		return fail("load database", err)
	}
	skip := func(reason, msg string) bulkDatabaseResult {
		res.Status, res.Reason, res.Message = bulkStatusSkipped, reason, msg
		return res
	}
	switch req.Action {
	case dbBulkStart:
		if !db.Suspended {
			return skip(dbSkipRunning, "already running")
		}
	case dbBulkStop:
		if db.Suspended {
			return skip(dbSkipStopped, "already stopped")
		}
		ref, err := rt.environments.EnvironmentOfDatabase(ctx, name)
		if err != nil {
			return fail("environment check", err)
		}
		if ref != nil && ref.Protected {
			res.Status, res.Message = dbBulkProtected, "in protected environment "+ref.Name+": stop it individually"
			return res
		}
	case dbBulkBackup:
		if !store.EngineSupportsBackups(db.Engine) {
			return skip(dbSkipNoBackups, "backups are not available for "+db.Engine)
		}
		if db.BackupTargetID == "" {
			return skip(dbSkipNoTarget, "no backup target")
		}
	}
	if req.DryRun {
		res.Status = dbBulkPlanned
		return res
	}
	switch req.Action {
	case dbBulkStart, dbBulkStop:
		if err := rt.databases.UpdateDatabaseSuspended(ctx, name, req.Action == dbBulkStop); errors.Is(err, store.ErrDatabaseNotFound) {
			res.Status, res.Message = bulkStatusNotFound, "database not found"
			return res
		} else if err != nil {
			return fail(req.Action, err)
		}
	case dbBulkBackup:
		if msg := rt.bulkBackupBlocked(ctx, db.BackupTargetID); msg != "" {
			res.Status, res.Message = bulkStatusError, msg
			return res
		}
		historyID, err := randomBackupHistoryID()
		if err != nil {
			return fail("backup id", err)
		}
		enqueue(dbBackupJob{historyID: historyID, name: name, engine: db.Engine, target: db.BackupTargetID})
		res.Message = "backup queued"
		res.BackupID = historyID
	}
	res.Status = bulkStatusOK
	return res
}

// bulkBackupBlocked returns why a backup cannot start now, or "" when it can.
func (rt *Router) bulkBackupBlocked(ctx context.Context, targetID string) string {
	if rt.backupRunner == nil {
		return "backups are not configured on this control plane (no master key set)"
	}
	if err := rt.diskGuard.Refuse(diskguard.OpBackup); err != nil {
		return err.Error()
	}
	if _, err := rt.backupTargets.GetBackupTarget(ctx, targetID); errors.Is(err, store.ErrBackupTargetNotFound) {
		return "backup target not found"
	} else if err != nil {
		rt.logger.Error("api: bulk backup: load target failed", slog.String("error", err.Error()), slog.String("target_id", targetID))
		return "backup target lookup failed"
	}
	if err := backup.CheckWorkDirSpace(); err != nil {
		return err.Error()
	}
	return ""
}

var dbBulkAuditStatus = map[string]int{
	bulkStatusOK:       http.StatusOK,
	bulkStatusSkipped:  http.StatusConflict,
	dbBulkProtected:    http.StatusConflict,
	bulkStatusDenied:   http.StatusForbidden,
	bulkStatusNotFound: http.StatusNotFound,
	bulkStatusError:    http.StatusInternalServerError,
}

// recordDatabaseBulkAudit writes one audit row per database, in the shape of
// the single-database route, since route audit only sees POST /databases/bulk.
func (rt *Router) recordDatabaseBulkAudit(ctx context.Context, r *http.Request, p bulkPrincipal, ability, action string, res bulkDatabaseResult) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: bulk databases audit id failed", slog.String("error", err.Error()))
		return
	}
	suffix := "/" + action
	if action == dbBulkBackup {
		suffix = "/backups"
	}
	entry := store.AuditEntry{
		ID: id, ActorType: p.actorType, ActorID: p.actorID,
		ActorName: rt.auditActorName(ctx, p.actorType, p.actorID, p.actorID),
		Ability:   ability, Method: http.MethodPost,
		Path:       "/api/v1/databases/" + res.Name + suffix,
		StatusCode: dbBulkAuditStatus[res.Status], RemoteAddr: clientIP(r),
		CreatedAt:  store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: bulk databases audit save failed", slog.String("error", err.Error()), slog.String("name", res.Name))
	}
}
