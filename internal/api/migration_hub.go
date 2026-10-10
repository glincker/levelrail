package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Hub steps, in order. Connect is the form before a session exists.
const (
	hubStepInventory = "inventory"
	hubStepPreflight = "preflight"
	hubStepCopy      = "copy"
	hubStepVerify    = "verify"
	hubStepCutover   = "cutover"
)

var hubStepOrder = []string{hubStepInventory, hubStepPreflight, hubStepCopy, hubStepVerify, hubStepCutover}

const maxHubBody = 16 << 10

// MigrationHubStore is the store surface the migration hub routes need.
type MigrationHubStore interface {
	CreateMigrationSession(ctx context.Context, s store.MigrationSession, items []store.MigrationItem) error
	GetMigrationSession(ctx context.Context, id string) (store.MigrationSession, error)
	ListMigrationSessions(ctx context.Context) ([]store.MigrationSession, error)
	SetMigrationStep(ctx context.Context, id, step string, now time.Time) error
	DeleteMigrationSession(ctx context.Context, id string) error
	ListMigrationItems(ctx context.Context, sessionID string) ([]store.MigrationItem, error)
	SetMigrationItemPlan(ctx context.Context, sessionID, sourceDB, targetName, targetVersion string, selected bool) error
	MarkMigrationItemTargetCreated(ctx context.Context, sessionID, sourceDB, version string) error
	SetMigrationItemStatus(ctx context.Context, sessionID, sourceDB, status, reason string, checked, mismatched int, detail string, now time.Time) error
}

type hubItemResource struct {
	SourceDB      string                      `json:"source_db"`
	SizeBytes     int64                       `json:"size_bytes"`
	Tables        int                         `json:"tables"`
	Extensions    []string                    `json:"extensions,omitempty"`
	TargetName    string                      `json:"target_name"`
	TargetVersion string                      `json:"target_version"`
	Selected      bool                        `json:"selected"`
	Status        string                      `json:"status"`
	Reason        string                      `json:"reason,omitempty"`
	Preflight     datamigrate.PreflightResult `json:"preflight"`
	Checked       int                         `json:"checked"`
	Mismatched    int                         `json:"mismatched"`
	TableCounts   []datamigrate.TableCount    `json:"table_counts,omitempty"`
	StartedAt     string                      `json:"started_at,omitempty"`
	FinishedAt    string                      `json:"finished_at,omitempty"`
}

type hubSummary struct {
	Databases       int   `json:"databases"`
	Selected        int   `json:"selected"`
	Blocked         int   `json:"blocked"`
	Warnings        int   `json:"warnings"`
	Verified        int   `json:"verified"`
	Failed          int   `json:"failed"`
	Copying         int   `json:"copying"`
	TotalBytes      int64 `json:"total_bytes"`
	RequiredBytes   int64 `json:"required_bytes"`
	EstimateSeconds int   `json:"estimate_seconds"`
	CanApply        bool  `json:"can_apply"`
}

type hubSessionResource struct {
	ID            string            `json:"id"`
	Engine        string            `json:"engine"`
	Host          string            `json:"host"`
	Port          int               `json:"port"`
	User          string            `json:"user,omitempty"`
	TLS           bool              `json:"tls,omitempty"`
	NodeID        string            `json:"node_id,omitempty"`
	Container     string            `json:"source_container,omitempty"`
	HelperNetwork string            `json:"helper_network,omitempty"`
	ServerVersion string            `json:"server_version"`
	FreeBytes     int64             `json:"free_bytes"`
	Step          string            `json:"step"`
	PasswordHeld  bool              `json:"password_held"`
	Running       bool              `json:"running"`
	Items         []hubItemResource `json:"items"`
	Summary       hubSummary        `json:"summary"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
}

type createHubSessionRequest struct {
	Engine   string `json:"engine"`
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	NodeID   string `json:"node_id,omitempty"`
	// Container names a database container on the node's Docker daemon. The
	// helper then joins that container's network and host/port are ignored.
	Container string `json:"container,omitempty"`
}

func newHubID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return "mig-" + hex.EncodeToString(b), nil
}

// defaultTargetVersion picks the managed version matching the source server.
func defaultTargetVersion(engine, version string, major int) string {
	if engine == datamigrate.EnginePostgres {
		if major > 0 {
			return fmt.Sprint(major)
		}
		return datamigrate.DefaultHelperVersion
	}
	parts := strings.Split(version, ".")
	if len(parts) >= 2 && parts[0] != "" {
		return parts[0] + "." + parts[1]
	}
	if version != "" {
		return version
	}
	return "latest"
}

func sourceFromSession(s store.MigrationSession, password string) datamigrate.Source {
	return datamigrate.Source{Host: s.Host, Port: s.Port, User: s.User, Password: password, TLS: s.TLS}
}

// handleCreateHubSession handles POST /api/v1/migration/hub/sessions: it
// inventories the source server read-only and stores the plan, never the password.
func (rt *Router) handleCreateHubSession(w http.ResponseWriter, r *http.Request) {
	var req createHubSessionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHubBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Container == "" && !datamigrate.InventorySupported(req.Engine) {
		writeError(w, http.StatusBadRequest, "server import supports postgres, mysql, mariadb and mongodb")
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "migrating a server is not available on this control plane")
		return
	}
	runtime, err := rt.execRuntime(req.NodeID)
	if err != nil {
		rt.logger.Error("api: migration hub: resolve node runtime failed", slog.String("error", err.Error()), slog.String("node_id", req.NodeID))
		writeError(w, http.StatusBadGateway, "the target node is not currently reachable")
		return
	}
	helperNetwork := ""
	if req.Container != "" {
		local, err := rt.resolveLocalSource(r.Context(), runtime, req.Container)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Engine, req.Host, req.Port, helperNetwork = local.Engine, local.Host, local.Port, local.Network
	}
	src := datamigrate.Source{Host: req.Host, Port: req.Port, User: req.User, Password: req.Password, TLS: req.TLS, Database: "inventory"}
	if err := src.Validate(req.Engine); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	src.Database = ""
	ctx, cancel := context.WithTimeout(r.Context(), envDuration(envHubInventoryTime, 2*time.Minute))
	defer cancel()
	insp := &datamigrate.Inspector{Runtime: runtime, Logger: rt.logger, HelperVersion: envOr(envHubHelperVersion, datamigrate.DefaultHelperVersion), HelperNetwork: helperNetwork}
	inv, err := insp.Inventory(ctx, req.Engine, src)
	if err != nil {
		rt.logger.Warn("api: migration hub: inventory failed", slog.String("error", err.Error()), slog.String("host", src.Host))
		writeError(w, http.StatusBadGateway, "could not read the source server: "+datamigrate.ExplainReachFailure(err.Error(), req.Container != ""))
		return
	}
	id, err := newHubID()
	if err != nil {
		rt.internalError(w, "api: migration hub: id failed", err)
		return
	}
	sess := store.MigrationSession{
		ID: id, Engine: req.Engine, Host: src.Host, Port: src.Port, User: src.User, TLS: src.TLS, NodeID: req.NodeID, HelperNetwork: helperNetwork, SourceContainer: req.Container,
		ServerVersion: inv.Version, ServerMajor: inv.Major, FreeBytes: inv.FreeBytes, Step: hubStepInventory, CreatedAt: time.Now(),
	}
	items, err := rt.hubInitialItems(r.Context(), sess, inv)
	if err != nil {
		rt.internalError(w, "api: migration hub: list databases failed", err)
		return
	}
	if err := rt.migrationHub.CreateMigrationSession(r.Context(), sess, items); err != nil {
		rt.internalError(w, "api: migration hub: save session failed", err, slog.String("session_id", id))
		return
	}
	rt.hubState.setPassword(id, req.Password)
	rt.logger.Info("api: migration hub session created", slog.String("session_id", id), slog.String("engine", req.Engine), slog.Int("databases", len(items)))
	rt.writeHubSession(w, r, http.StatusCreated, id)
}

func envOr(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

func (rt *Router) hubInitialItems(ctx context.Context, sess store.MigrationSession, inv datamigrate.Inventory) ([]store.MigrationItem, error) {
	taken, err := rt.hubTakenNames(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(inv.Databases))
	for _, d := range inv.Databases {
		names = append(names, d.Name)
	}
	targets := datamigrate.DeriveTargetNames(names, taken)
	version := defaultTargetVersion(sess.Engine, inv.Version, inv.Major)
	items := make([]store.MigrationItem, 0, len(inv.Databases))
	for _, d := range inv.Databases {
		items = append(items, store.MigrationItem{
			SourceDB: d.Name, SizeBytes: d.SizeBytes, Tables: d.Tables, Extensions: d.Extensions,
			TargetName: targets[d.Name], TargetVersion: version, Selected: d.Tables > 0,
		})
	}
	return items, nil
}

func (rt *Router) hubTakenNames(ctx context.Context) (map[string]bool, error) {
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	taken := make(map[string]bool, len(dbs))
	for _, d := range dbs {
		taken[d.Name] = true
	}
	return taken, nil
}

// hubView builds the wire shape, recomputing preflight from stored facts.
func (rt *Router) hubView(ctx context.Context, sess store.MigrationSession, items []store.MigrationItem) (hubSessionResource, error) {
	taken, err := rt.hubTakenNames(ctx)
	if err != nil {
		return hubSessionResource{}, err
	}
	_, held := rt.hubState.password(sess.ID)
	running := rt.hubState.isRunning(sess.ID)
	res := hubSessionResource{
		ID: sess.ID, Engine: sess.Engine, Host: sess.Host, Port: sess.Port, User: sess.User, TLS: sess.TLS, NodeID: sess.NodeID,
		Container: sess.SourceContainer, HelperNetwork: sess.HelperNetwork,
		ServerVersion: sess.ServerVersion, FreeBytes: sess.FreeBytes, Step: sess.Step, PasswordHeld: held, Running: running,
		CreatedAt: sess.CreatedAt.Format(time.RFC3339), UpdatedAt: sess.UpdatedAt.Format(time.RFC3339),
		Items: make([]hubItemResource, 0, len(items)),
	}
	margin, mbps := envFloat(envHubDiskMargin, 1.5), envFloat(envHubMBPerSecond, 20)
	sum := &res.Summary
	remainingFree := sess.FreeBytes
	for _, it := range items {
		status, reason := it.Status, it.Reason
		if status == store.HubItemCopying && !running {
			status, reason = store.HubItemFailed, "The copy was interrupted. Run it again, it replaces the target's contents."
		}
		pre := datamigrate.Preflight(datamigrate.PreflightInput{
			Engine: sess.Engine, DB: datamigrate.DatabaseInfo{Name: it.SourceDB, SizeBytes: it.SizeBytes, Tables: it.Tables, Extensions: it.Extensions},
			SourceMajor: sess.ServerMajor, TargetName: it.TargetName, TargetVersion: it.TargetVersion,
			TargetExists: taken[it.TargetName], OwnTarget: it.CreatedTarget,
			FreeBytes: remainingFree, DiskMargin: margin, MBPerSecond: mbps,
		})
		src := sourceFromSession(sess, "")
		src.Database = it.SourceDB
		if err := src.Validate(sess.Engine); err != nil {
			pre.Checks = append(pre.Checks, datamigrate.PreflightCheck{ID: "source_name", Severity: datamigrate.SeverityBlock,
				Message:    "The source database name cannot be copied automatically: " + err.Error(),
				NextAction: "Leave this database out and move it with a manual dump and restore."})
			pre.Blocked = true
		}
		if it.Selected && status != store.HubItemVerified && remainingFree >= 0 {
			remainingFree -= pre.RequiredBytes
			if remainingFree < 0 {
				remainingFree = 0
			}
		}
		out := hubItemResource{
			SourceDB: it.SourceDB, SizeBytes: it.SizeBytes, Tables: it.Tables, Extensions: it.Extensions,
			TargetName: it.TargetName, TargetVersion: pre.TargetVersion, Selected: it.Selected, Status: status, Reason: reason,
			Preflight: pre, Checked: it.Checked, Mismatched: it.Mismatched,
		}
		if it.Detail != "" {
			var v datamigrate.Verification
			if json.Unmarshal([]byte(it.Detail), &v) == nil {
				out.TableCounts = v.Tables
			}
		}
		if !it.StartedAt.IsZero() {
			out.StartedAt = it.StartedAt.Format(time.RFC3339)
		}
		if !it.FinishedAt.IsZero() {
			out.FinishedAt = it.FinishedAt.Format(time.RFC3339)
		}
		res.Items = append(res.Items, out)
		sum.Databases++
		sum.TotalBytes += it.SizeBytes
		if !it.Selected {
			continue
		}
		sum.Selected++
		sum.RequiredBytes += pre.RequiredBytes
		sum.EstimateSeconds += pre.EstimateSeconds
		if pre.Blocked && status != store.HubItemVerified {
			sum.Blocked++
		}
		for _, c := range pre.Checks {
			if c.Severity == datamigrate.SeverityWarn {
				sum.Warnings++
			}
		}
		switch status {
		case store.HubItemVerified:
			sum.Verified++
		case store.HubItemFailed:
			sum.Failed++
		case store.HubItemCopying:
			sum.Copying++
		}
	}
	sum.CanApply = sum.Selected > 0 && sum.Blocked == 0 && !running && sum.Verified < sum.Selected
	return res, nil
}

func (rt *Router) writeHubSession(w http.ResponseWriter, r *http.Request, status int, id string) {
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
	writeJSON(w, status, view)
}

// handleGetHubSession handles GET /api/v1/migration/hub/sessions/{id}.
func (rt *Router) handleGetHubSession(w http.ResponseWriter, r *http.Request) {
	rt.writeHubSession(w, r, http.StatusOK, r.PathValue("id"))
}

// handleListHubSessions handles GET /api/v1/migration/hub/sessions.
func (rt *Router) handleListHubSessions(w http.ResponseWriter, r *http.Request) {
	list, err := rt.migrationHub.ListMigrationSessions(r.Context())
	if err != nil {
		rt.internalError(w, "api: migration hub: list sessions failed", err)
		return
	}
	out := make([]hubSessionResource, 0, len(list))
	for _, s := range list {
		items, err := rt.migrationHub.ListMigrationItems(r.Context(), s.ID)
		if err != nil {
			rt.internalError(w, "api: migration hub: list items failed", err, slog.String("session_id", s.ID))
			return
		}
		v, err := rt.hubView(r.Context(), s, items)
		if err != nil {
			rt.internalError(w, "api: migration hub: build plan failed", err, slog.String("session_id", s.ID))
			return
		}
		v.Items = nil
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteHubSession handles DELETE /api/v1/migration/hub/sessions/{id}.
// It forgets the plan and password only: the source and the managed
// databases already created are left alone.
func (rt *Router) handleDeleteHubSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if rt.hubState.isRunning(id) {
		writeError(w, http.StatusConflict, "a copy is running for this session, wait for it to finish")
		return
	}
	if err := rt.migrationHub.DeleteMigrationSession(r.Context(), id); err != nil {
		rt.internalError(w, "api: migration hub: delete session failed", err, slog.String("session_id", id))
		return
	}
	rt.hubState.forget(id)
	w.WriteHeader(http.StatusNoContent)
}

type hubSelectionItem struct {
	SourceDB      string  `json:"source_db"`
	Selected      *bool   `json:"selected,omitempty"`
	TargetName    *string `json:"target_name,omitempty"`
	TargetVersion *string `json:"target_version,omitempty"`
}

type hubSelectionRequest struct {
	Items []hubSelectionItem `json:"items"`
}

// handlePutHubSelection handles PUT /api/v1/migration/hub/sessions/{id}/selection.
func (rt *Router) handlePutHubSelection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req hubSelectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if rt.hubState.isRunning(id) {
		writeError(w, http.StatusConflict, "a copy is running for this session")
		return
	}
	items, err := rt.migrationHub.ListMigrationItems(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: migration hub: load items failed", err, slog.String("session_id", id))
		return
	}
	bySource := make(map[string]store.MigrationItem, len(items))
	for _, it := range items {
		bySource[it.SourceDB] = it
	}
	for _, in := range req.Items {
		it, ok := bySource[in.SourceDB]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown source database "+in.SourceDB)
			return
		}
		if in.Selected != nil {
			it.Selected = *in.Selected
		}
		if in.TargetName != nil {
			it.TargetName = strings.TrimSpace(*in.TargetName)
		}
		if in.TargetVersion != nil {
			it.TargetVersion = strings.TrimSpace(*in.TargetVersion)
		}
		if err := rt.migrationHub.SetMigrationItemPlan(r.Context(), id, it.SourceDB, it.TargetName, it.TargetVersion, it.Selected); err != nil {
			rt.internalError(w, "api: migration hub: save selection failed", err, slog.String("session_id", id))
			return
		}
	}
	rt.writeHubSession(w, r, http.StatusOK, id)
}

type hubStepRequest struct {
	Step string `json:"step"`
}

// handlePutHubStep handles PUT /api/v1/migration/hub/sessions/{id}/step.
// It only moves forward through steps whose prerequisites hold.
func (rt *Router) handlePutHubStep(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req hubStepRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHubBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	idx := stepIndex(req.Step)
	if idx < 0 || req.Step == hubStepCopy {
		writeError(w, http.StatusBadRequest, "step must be one of inventory, preflight, verify, cutover")
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
	if idx <= stepIndex(sess.Step) {
		rt.writeHubSession(w, r, http.StatusOK, id)
		return
	}
	if req.Step == hubStepVerify || req.Step == hubStepCutover {
		items, _ := rt.migrationHub.ListMigrationItems(r.Context(), id)
		verified := 0
		for _, it := range items {
			if it.Status == store.HubItemVerified {
				verified++
			}
		}
		if verified == 0 {
			writeError(w, http.StatusConflict, "no database is verified yet, run the copy first")
			return
		}
	}
	if err := rt.migrationHub.SetMigrationStep(r.Context(), id, req.Step, time.Now()); err != nil {
		rt.internalError(w, "api: migration hub: set step failed", err, slog.String("session_id", id))
		return
	}
	rt.writeHubSession(w, r, http.StatusOK, id)
}

func stepIndex(step string) int {
	for i, s := range hubStepOrder {
		if s == step {
			return i
		}
	}
	return -1
}
