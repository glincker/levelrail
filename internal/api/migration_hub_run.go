package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const hubReadyPoll = 2 * time.Second

type hubApplyRequest struct {
	Password string `json:"password,omitempty"`
}

// handleApplyHubSession handles POST /api/v1/migration/hub/sessions/{id}/apply.
// It creates one managed database per selected source database and copies
// them with bounded concurrency. The source is only ever read.
func (rt *Router) handleApplyHubSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req hubApplyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHubBody)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sess, err := rt.migrationHub.GetMigrationSession(r.Context(), id)
	if errors.Is(err, store.ErrMigrationSessionNotFound) {
		writeError(w, http.StatusNotFound, "migration session not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: migration hub: load session failed", err, slog.String("session_id", id))
		return
	}
	items, err := rt.migrationHub.ListMigrationItems(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: migration hub: load items failed", err, slog.String("session_id", id))
		return
	}
	view, err := rt.hubView(r.Context(), sess, items)
	if err != nil {
		rt.internalError(w, "api: migration hub: build plan failed", err, slog.String("session_id", id))
		return
	}
	switch {
	case view.Summary.Selected == 0:
		writeError(w, http.StatusBadRequest, "select at least one database")
		return
	case view.Summary.Blocked > 0:
		writeError(w, http.StatusConflict, "some selected databases are blocked by preflight, fix or deselect them first")
		return
	}
	if req.Password != "" {
		rt.hubState.setPassword(id, req.Password)
	}
	password, held := rt.hubState.password(id)
	if !held {
		writeError(w, http.StatusConflict, "the source password is no longer held in memory, enter it again to continue")
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "copying data is not available on this control plane")
		return
	}
	runtime, err := rt.execRuntime(sess.NodeID)
	if err != nil {
		rt.logger.Error("api: migration hub: resolve node runtime failed", slog.String("error", err.Error()), slog.String("session_id", id))
		writeError(w, http.StatusBadGateway, "the target node is not currently reachable")
		return
	}
	if !rt.hubState.begin(id) {
		writeError(w, http.StatusConflict, "a copy is already running for this session")
		return
	}

	var todo []store.MigrationItem
	byName := map[string]hubItemResource{}
	for _, v := range view.Items {
		byName[v.SourceDB] = v
	}
	for _, it := range items {
		if !it.Selected || it.Status == store.HubItemVerified {
			continue
		}
		it.TargetVersion = byName[it.SourceDB].TargetVersion
		if err := rt.hubEnsureTarget(r.Context(), sess, &it); err != nil {
			rt.hubState.end(id)
			rt.internalError(w, "api: migration hub: create database failed", err, slog.String("session_id", id), slog.String("database", it.TargetName))
			return
		}
		if err := rt.migrationHub.SetMigrationItemStatus(r.Context(), id, it.SourceDB, store.HubItemPending, "", 0, 0, "", time.Now()); err != nil {
			rt.hubState.end(id)
			rt.internalError(w, "api: migration hub: reset item failed", err, slog.String("session_id", id))
			return
		}
		todo = append(todo, it)
	}
	if err := rt.migrationHub.SetMigrationStep(r.Context(), id, hubStepCopy, time.Now()); err != nil {
		rt.logger.Warn("api: migration hub: set step failed", slog.String("error", err.Error()), slog.String("session_id", id))
	}
	rt.nudgeReconciler()
	src := sourceFromSession(sess, password)
	go rt.runHubCopies(sess, src, todo, runtime) //nolint:gosec // detached on purpose: r.Context() is cancelled when this handler returns
	rt.writeHubSession(w, r, http.StatusAccepted, id)
}

// hubEnsureTarget creates the managed database for an item when it does not
// exist yet. An existing one is only reused if this migration created it.
func (rt *Router) hubEnsureTarget(ctx context.Context, sess store.MigrationSession, it *store.MigrationItem) error {
	if _, err := rt.databases.GetDesiredDatabase(ctx, it.TargetName); err == nil {
		if !it.CreatedTarget {
			return fmt.Errorf("database %q already exists and was not created by this migration", it.TargetName)
		}
		return nil
	} else if !errors.Is(err, store.ErrDatabaseNotFound) {
		return fmt.Errorf("check database %q: %w", it.TargetName, err)
	}
	d := store.DesiredDatabase{Name: it.TargetName, Engine: sess.Engine, Version: it.TargetVersion}
	if err := rt.databases.SaveDesiredDatabase(ctx, d); err != nil {
		return fmt.Errorf("create database %q: %w", it.TargetName, err)
	}
	if sess.NodeID != "" {
		if err := rt.databases.UpdateDatabaseNode(ctx, it.TargetName, sess.NodeID); err != nil {
			return fmt.Errorf("place database %q: %w", it.TargetName, err)
		}
	}
	if err := rt.migrationHub.MarkMigrationItemTargetCreated(ctx, sess.ID, it.SourceDB, it.TargetVersion); err != nil {
		return fmt.Errorf("record database %q: %w", it.TargetName, err)
	}
	it.CreatedTarget = true
	return nil
}

// runHubCopies runs detached. One database failing never stops the others.
func (rt *Router) runHubCopies(sess store.MigrationSession, src datamigrate.Source, items []store.MigrationItem, runtime docker.Runtime) {
	defer rt.hubState.end(sess.ID)
	sem := make(chan struct{}, envInt(envHubConcurrency, 2))
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			rt.copyHubItem(sess, src, it, runtime)
		}()
	}
	wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rt.migrationHub.SetMigrationStep(ctx, sess.ID, hubStepVerify, time.Now()); err != nil {
		rt.logger.Warn("api: migration hub: set step failed", slog.String("error", err.Error()), slog.String("session_id", sess.ID))
	}
}

func (rt *Router) copyHubItem(sess store.MigrationSession, src datamigrate.Source, it store.MigrationItem, runtime docker.Runtime) {
	log := rt.logger.With(slog.String("session_id", sess.ID), slog.String("database", it.TargetName), slog.String("source_db", it.SourceDB))
	timeout := copyTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	set := func(status, reason string, v datamigrate.Verification) {
		detail, _ := json.Marshal(v)
		fctx, fcancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer fcancel()
		if err := rt.migrationHub.SetMigrationItemStatus(fctx, sess.ID, it.SourceDB, status, reason, v.Checked, v.Mismatched, string(detail), time.Now()); err != nil {
			log.Error("api: migration hub: record item status failed", slog.String("error", err.Error()))
		}
	}
	fail := func(reason string) {
		log.Warn("api: migration hub: copy failed", slog.String("reason", reason))
		set(store.HubItemFailed, src.Scrub(reason), datamigrate.Verification{})
	}

	set(store.HubItemCopying, "", datamigrate.Verification{})
	if err := waitTargetRunning(ctx, runtime, it.TargetName, envDuration(envHubReadyTimeout, 5*time.Minute)); err != nil {
		fail(err.Error())
		return
	}
	item := src
	item.Database = it.SourceDB
	err := rt.dataImports.ClaimDatabaseDataImport(ctx, it.TargetName, src.Host, src.Port, it.SourceDB, time.Now(), timeout)
	if err != nil {
		fail(err.Error())
		return
	}
	copier := &datamigrate.Copier{Runtime: runtime, Restorer: &backup.ContainerRestorer{Runtime: runtime}, Logger: rt.logger, HelperNetwork: sess.HelperNetwork}
	v, err := copier.Copy(ctx, datamigrate.Target{Name: it.TargetName, Engine: sess.Engine, Version: it.TargetVersion}, item)
	status, reason := store.DataImportVerified, ""
	hubStatus := store.HubItemVerified
	if err != nil {
		status, reason, hubStatus = store.DataImportFailed, datamigrate.ExplainReachFailure(err.Error(), sess.SourceContainer != ""), store.HubItemFailed
	}
	detail, _ := json.Marshal(v)
	fctx, fcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer fcancel()
	if ferr := rt.dataImports.FinishDatabaseDataImport(fctx, it.TargetName, status, reason, v.Checked, v.Mismatched, string(detail), time.Now()); ferr != nil {
		log.Error("api: migration hub: record data import failed", slog.String("error", ferr.Error()))
	}
	if err != nil {
		log.Warn("api: migration hub: copy failed", slog.String("reason", reason))
	} else {
		log.Info("api: migration hub: copy verified", slog.Int("tables", v.Checked))
	}
	set(hubStatus, reason, v)
}

func waitTargetRunning(ctx context.Context, rt docker.Runtime, dbName string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		st, err := rt.InspectByName(ctx, database.ContainerName(dbName))
		if err == nil && st != nil && st.Running {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("the managed database did not become ready in time, check its status and run the copy again")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for the managed database: %w", ctx.Err())
		case <-time.After(hubReadyPoll):
		}
	}
}

type hubEnvVar struct {
	EnvVar string `json:"env_var"`
	Field  string `json:"field"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

type hubConnection struct {
	Database  string      `json:"database"`
	Engine    string      `json:"engine"`
	Host      string      `json:"host"`
	Port      int         `json:"port"`
	Username  string      `json:"username,omitempty"`
	Name      string      `json:"name,omitempty"`
	URL       string      `json:"url"`
	Env       []hubEnvVar `json:"env"`
	Reference string      `json:"reference"`
	Revealed  bool        `json:"revealed"`
}

// handleHubConnection handles GET .../items/{db}/connection: the connection
// details without any secret. The password is only in the reveal route.
func (rt *Router) handleHubConnection(w http.ResponseWriter, r *http.Request) {
	rt.writeHubConnection(w, r, false)
}

// handleHubReveal handles POST .../items/{db}/reveal. It needs
// read:sensitive, so the audit log records who revealed the secret.
func (rt *Router) handleHubReveal(w http.ResponseWriter, r *http.Request) {
	rt.writeHubConnection(w, r, true)
}

func (rt *Router) writeHubConnection(w http.ResponseWriter, r *http.Request, reveal bool) {
	id, source := r.PathValue("id"), r.PathValue("db")
	items, err := rt.migrationHub.ListMigrationItems(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: migration hub: load items failed", err, slog.String("session_id", id))
		return
	}
	var item *store.MigrationItem
	for i := range items {
		if items[i].SourceDB == source {
			item = &items[i]
		}
	}
	if item == nil || item.Status != store.HubItemVerified {
		writeError(w, http.StatusNotFound, "no verified database for this source database yet")
		return
	}
	sess, err := rt.migrationHub.GetMigrationSession(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: migration hub: load session failed", err, slog.String("session_id", id))
		return
	}
	conn, ok := rt.buildHubConnection(r.Context(), sess.Engine, item.TargetName, reveal)
	if !ok {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane")
		return
	}
	if reveal {
		rt.logger.Info("api: migration hub: connection secret revealed", slog.String("session_id", id), slog.String("database", item.TargetName))
	}
	writeJSON(w, http.StatusOK, conn)
}
