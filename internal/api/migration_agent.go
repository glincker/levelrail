package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/migrateagent"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MigrationAgentService is the control plane side of the read-only migration
// agent. *migrateagent.Service satisfies it.
type MigrationAgentService interface {
	Create(ctx context.Context, label, actor string) (migrateagent.Created, error)
	List(ctx context.Context) ([]migrateagent.SessionView, error)
	Get(ctx context.Context, id string) (migrateagent.SessionView, migrateagent.InventoryDoc, migrateagent.ScanDoc, error)
	Revoke(ctx context.Context, id, actor string) error
	Discover(ctx context.Context, id string) (migrateagent.InventoryDoc, error)
	Scan(ctx context.Context, id string, targets []string) (migrateagent.ScanDoc, error)
	Auditor() *migrateagent.Auditor
	Config() migrateagent.Config
}

// SetMigrationAgent enables the /api/v1/migration/agent routes.
func (rt *Router) SetMigrationAgent(s MigrationAgentService) { rt.migrationAgent = s }

func (rt *Router) migrationAgentOrError(w http.ResponseWriter) (MigrationAgentService, bool) {
	if rt.migrationAgent == nil {
		writeError(w, http.StatusNotImplemented, "the migration agent is not available on this control plane")
		return nil, false
	}
	return rt.migrationAgent, true
}

func (rt *Router) writeMigrationAgentError(w http.ResponseWriter, msg string, err error) {
	switch {
	case errors.Is(err, store.ErrMigrationAgentNotFound):
		writeError(w, http.StatusNotFound, "no migration agent session with that id")
	case errors.Is(err, migrateagent.ErrOffline), errors.Is(err, migrateagent.ErrNotActive):
		writeError(w, http.StatusConflict, err.Error())
	default:
		rt.internalError(w, "api: migration agent: "+msg, err)
	}
}

type createMigrationAgentRequest struct {
	Label            string `json:"label"`
	ControlPlaneAddr string `json:"control_plane_addr"`
}

type createMigrationAgentResponse struct {
	ID               string    `json:"id"`
	Token            string    `json:"token"`
	ExpiresAt        time.Time `json:"expires_at"`
	CAFingerprint    string    `json:"ca_fingerprint"`
	ControlPlaneAddr string    `json:"control_plane_addr,omitempty"`
	Binary           string    `json:"binary"`
	Command          string    `json:"command,omitempty"`
	SessionMaxHours  int       `json:"session_max_hours"`
}

// buildMigrationCommand renders the one-liner. The token is shown once and every
// value is validated or generated here, so nothing can inject shell syntax.
func buildMigrationCommand(binary, addr, token, caFingerprint string) string {
	return "sudo APP_CONTROL_PLANE_ADDR=" + addr + " APP_MIGRATION_TOKEN=" + token + " APP_CA_FINGERPRINT=" + caFingerprint + " " + binary + " install"
}

// handleCreateMigrationAgentSession handles POST /api/v1/migration/agent/sessions.
func (rt *Router) handleCreateMigrationAgentSession(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.migrationAgentOrError(w)
	if !ok {
		return
	}
	var req createMigrationAgentRequest
	if r.ContentLength != 0 {
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if len(req.Label) > 80 || strings.ContainsAny(req.Label, "\r\n") {
		writeError(w, http.StatusBadRequest, "label must be one line of at most 80 characters")
		return
	}
	addr := req.ControlPlaneAddr
	if addr == "" {
		addr = rt.defaultControlPlaneAddr()
	}
	if addr != "" && !controlPlaneAddrRe.MatchString(addr) {
		writeError(w, http.StatusBadRequest, "control_plane_addr must be host:port")
		return
	}
	if rt.agentCAFingerprint == "" {
		writeError(w, http.StatusConflict, "the agent CA fingerprint is not available, so a pinned install command cannot be built")
		return
	}
	_, actorID, actorName, _ := rt.currentActor(r)
	created, err := svc.Create(r.Context(), req.Label, firstNonEmpty(actorName, actorID))
	if err != nil {
		rt.writeMigrationAgentError(w, "create session", err)
		return
	}
	binary := rt.brand.MigrateName()
	resp := createMigrationAgentResponse{ID: created.ID, Token: created.Token, ExpiresAt: created.ExpiresAt,
		CAFingerprint: rt.agentCAFingerprint, ControlPlaneAddr: addr, Binary: binary, SessionMaxHours: int(svc.Config().SessionMax.Hours())}
	if addr != "" {
		resp.Command = buildMigrationCommand(binary, addr, created.Token, rt.agentCAFingerprint)
	}
	rt.logger.Info("api: migration agent token issued", slog.String("session_id", created.ID))
	writeJSON(w, http.StatusCreated, resp)
}

// handleListMigrationAgentSessions handles GET /api/v1/migration/agent/sessions.
func (rt *Router) handleListMigrationAgentSessions(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.migrationAgentOrError(w)
	if !ok {
		return
	}
	rows, err := svc.List(r.Context())
	if err != nil {
		rt.writeMigrationAgentError(w, "list sessions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": rows})
}

type migrationAgentDetail struct {
	Session   migrateagent.SessionView  `json:"session"`
	Inventory migrateagent.InventoryDoc `json:"inventory"`
	Scan      migrateagent.ScanDoc      `json:"scan"`
}

func (rt *Router) importedRefs(r *http.Request) []migrateagent.ImportedRef {
	apps, err := rt.importedChecklistApps(r)
	if err != nil {
		return nil
	}
	refs := make([]migrateagent.ImportedRef, 0, len(apps))
	for _, a := range apps {
		_, sid, _ := strings.Cut(a.Labels[platformimport.LabelSourceID], ":")
		refs = append(refs, migrateagent.ImportedRef{App: a.Name, SourceID: sid})
	}
	return refs
}

// handleGetMigrationAgentSession handles GET /api/v1/migration/agent/sessions/{id}.
func (rt *Router) handleGetMigrationAgentSession(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.migrationAgentOrError(w)
	if !ok {
		return
	}
	view, inv, scan, err := svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeMigrationAgentError(w, "get session", err)
		return
	}
	migrateagent.MarkUnlisted(&inv, rt.importedRefs(r))
	writeJSON(w, http.StatusOK, migrationAgentDetail{Session: view, Inventory: inv, Scan: scan})
}

// handleDiscoverMigrationAgent handles POST /api/v1/migration/agent/sessions/{id}/discover.
func (rt *Router) handleDiscoverMigrationAgent(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.migrationAgentOrError(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	inv, err := svc.Discover(r.Context(), id)
	if err != nil {
		rt.writeMigrationAgentError(w, "discover", err)
		return
	}
	migrateagent.MarkUnlisted(&inv, rt.importedRefs(r))
	writeJSON(w, http.StatusOK, inv)
}

// handleRevokeMigrationAgentSession handles DELETE /api/v1/migration/agent/sessions/{id}.
func (rt *Router) handleRevokeMigrationAgentSession(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.migrationAgentOrError(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, _, _, err := svc.Get(r.Context(), id); err != nil {
		rt.writeMigrationAgentError(w, "revoke", err)
		return
	}
	_, actorID, actorName, _ := rt.currentActor(r)
	if err := svc.Revoke(r.Context(), id, firstNonEmpty(actorName, actorID)); err != nil {
		rt.writeMigrationAgentError(w, "revoke", err)
		return
	}
	rt.logger.Warn("api: migration agent session revoked", slog.String("session_id", id))
	w.WriteHeader(http.StatusNoContent)
}

type migrationVerifyTarget struct {
	Name  string `json:"name"`
	Files int64  `json:"files"`
	Bytes int64  `json:"bytes"`
}

type migrationVerifyRequest struct {
	App string `json:"app"`
	// TargetStats override the control plane's own measurement, for volumes that live on
	// another node. They are shown as operator-supplied and never auto-complete the step.
	TargetStats []migrationVerifyTarget `json:"target_stats,omitempty"`
	// AckEmptyVolumes accepts matched volumes that are empty on the source.
	AckEmptyVolumes bool `json:"ack_empty_volumes,omitempty"`
}

type migrationVerifyResponse struct {
	Comparisons []migrateagent.Comparison  `json:"comparisons"`
	Advance     migrateagent.AdvanceResult `json:"advance"`
	Databases   int                        `json:"databases_on_source"`
	Checklist   checklistView              `json:"checklist"`
}

func appVolumes(svc store.DesiredService) []migrateagent.AppVolume {
	var vols []migrateagent.AppVolume
	for _, v := range svc.Volumes {
		vols = append(vols, migrateagent.AppVolume{Name: v.Name, Source: migrateagent.SourceTarget(svc.Name, v.Name)})
	}
	for _, b := range svc.BindMounts {
		vols = append(vols, migrateagent.AppVolume{Name: b.HostPath, Source: b.HostPath})
	}
	return vols
}

// handleVerifyMigrationAgent handles POST /api/v1/migration/agent/sessions/{id}/verify:
// scan the source volumes of one imported app, compare them with this host and,
// on zero diffs, complete data_verified as the migration agent. Caller-supplied
// target numbers or empty volumes without an acknowledgement never auto-advance.
func (rt *Router) handleVerifyMigrationAgent(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.migrationAgentOrError(w)
	if !ok {
		return
	}
	var req migrationVerifyRequest
	if err := decodeJSONBody(r, &req); err != nil || req.App == "" {
		writeError(w, http.StatusBadRequest, "app is required")
		return
	}
	apps, err := rt.importedChecklistApps(r)
	if err != nil {
		rt.internalError(w, "api: migration agent verify: list apps failed", err)
		return
	}
	var app *store.DesiredService
	for i := range apps {
		if apps[i].Name == req.App {
			app = &apps[i]
		}
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "no imported app named "+req.App)
		return
	}
	vols := appVolumes(*app)
	if len(vols) == 0 {
		writeError(w, http.StatusConflict, "the app has no volumes to compare")
		return
	}
	ctx, id := r.Context(), r.PathValue("id")
	inv, err := svc.Discover(ctx, id)
	if err != nil {
		rt.writeMigrationAgentError(w, "discover before verify", err)
		return
	}
	targets := make([]string, 0, len(vols))
	for _, v := range vols {
		targets = append(targets, v.Source)
	}
	scan, err := svc.Scan(ctx, id, targets)
	if err != nil {
		rt.writeMigrationAgentError(w, "scan", err)
		return
	}
	override := map[string]migrateagent.TargetStat{}
	for _, t := range req.TargetStats {
		override[t.Name] = migrateagent.TargetStat{Files: t.Files, Bytes: t.Bytes}
	}
	measured := map[string]migrateagent.TargetStat{}
	for _, v := range vols {
		if o, ok := override[v.Name]; ok {
			o.Operator = true
			measured[v.Name] = o
			continue
		}
		measured[v.Name] = migrateagent.MeasureTarget(ctx, svc.Config().VolumesDir, v.Name)
	}
	cs := migrateagent.Compare(vols, scan.Stats, measured)
	dbs := 0
	for _, c := range inv.Containers {
		if c.DatabaseEngine != "" {
			dbs++
		}
	}
	resp, ok := rt.applyAgentVerification(w, r, svc, id, *app, cs, dbs, migrateagent.AdvanceOptions{AckEmpty: req.AckEmptyVolumes})
	if ok {
		writeJSON(w, http.StatusOK, resp)
	}
}

func (rt *Router) applyAgentVerification(w http.ResponseWriter, r *http.Request, svc MigrationAgentService, sessionID string, app store.DesiredService, cs []migrateagent.Comparison, dbs int, opts migrateagent.AdvanceOptions) (migrationVerifyResponse, bool) {
	ctx := r.Context()
	c, row, err := rt.loadChecklist(ctx, app)
	if err != nil {
		rt.internalError(w, "api: migration agent verify: load checklist failed", err, slog.String("app", app.Name))
		return migrationVerifyResponse{}, false
	}
	now := time.Now().UTC()
	adv := migrateagent.AutoAdvance(&c, cs, dbs, opts, now)
	view := buildChecklistView(app, row, c)
	if adv.Advanced {
		saved, err := rt.persistChecklist(ctx, app, row, c, now)
		if err != nil {
			rt.internalError(w, "api: migration agent verify: save checklist failed", err, slog.String("app", app.Name))
			return migrationVerifyResponse{}, false
		}
		view = buildChecklistView(app, &saved, c)
	}
	_, actorID, actorName, _ := rt.currentActor(r)
	actor := firstNonEmpty(actorName, actorID)
	matched := 0
	for _, x := range cs {
		if x.Status == migrateagent.StatusMatch {
			matched++
		}
	}
	aud := svc.Auditor()
	aud.Record(ctx, migrateagent.Event{Action: migrateagent.ActionVerified, SessionID: sessionID, Actor: actor,
		Detail: map[string]any{"app": app.Name, "volumes": len(cs), "matched": matched,
			"overrides": migrateagent.OverrideCount(cs), "ack_empty_volumes": opts.AckEmpty, "empty_volumes": migrateagent.EmptyCount(cs)}})
	if adv.Advanced {
		aud.Record(ctx, migrateagent.Event{Action: migrateagent.ActionStepAdvanced, SessionID: sessionID, Actor: migrateagent.Actor,
			Detail: map[string]any{"app": app.Name, "step": migrationplan.StepDataVerified, "triggered_by": actor}})
	}
	return migrationVerifyResponse{Comparisons: cs, Advance: adv, Databases: dbs, Checklist: view}, true
}

// auditChecklistReopen records an operator undoing a step the migration agent completed.
func (rt *Router) auditChecklistReopen(r *http.Request, app, step, action string, before migrationplan.StepState) {
	if rt.migrationAgent == nil || action != "reset" || before.Actor != migrateagent.Actor {
		return
	}
	_, actorID, actorName, _ := rt.currentActor(r)
	rt.migrationAgent.Auditor().Record(r.Context(), migrateagent.Event{Action: migrateagent.ActionStepReopened, SessionID: "-",
		Actor: firstNonEmpty(actorName, actorID), Detail: map[string]any{"app": app, "step": step}})
}
