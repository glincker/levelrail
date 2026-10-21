package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxWizardBody = 256 << 10

// MigrationChecklistStore is the store surface the cutover checklist needs.
type MigrationChecklistStore interface {
	SaveMigrationChecklist(ctx context.Context, c store.MigrationChecklist) error
	GetMigrationChecklist(ctx context.Context, app string) (store.MigrationChecklist, error)
	ListMigrationChecklists(ctx context.Context) ([]store.MigrationChecklist, error)
}

// wizardRequest is a platform import request plus per-item overrides. Like
// platformImportRequest it carries the source credential, never echoed.
type wizardRequest struct {
	platformImportRequest
	Overrides map[string]migrationplan.Override `json:"overrides,omitempty"`
	DryRun    bool                              `json:"dry_run,omitempty"`
}

func (rt *Router) decodeWizard(w http.ResponseWriter, r *http.Request) (wizardRequest, *platformimport.Discovery, bool) {
	var req wizardRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWizardBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, nil, false
	}
	src, ok := rt.openImportSource(w, req.platformImportRequest)
	if !ok {
		return req, nil, false
	}
	disc, err := src.Discover(r.Context())
	if err != nil {
		rt.writeImportError(w, fmt.Errorf("reading the source platform: %w", err), req.Token)
		return req, nil, false
	}
	return req, disc, true
}

func (rt *Router) analyzeWizard(ctx context.Context, req wizardRequest, disc *platformimport.Discovery) (*platformimport.Plan, migrationplan.Report, error) {
	opts, domains, err := rt.importPlanOptions(ctx, req.platformImportRequest)
	if err != nil {
		return nil, migrationplan.Report{}, err
	}
	plan, rep := migrationplan.Analyze(disc, migrationplan.Options{Plan: opts, TakenDomains: domains, Overrides: req.Overrides})
	return plan, rep, nil
}

// handleMigrationPlan handles POST /api/v1/migration/plan. It reads the
// source and creates nothing; the per-item preview is the dry-run diff.
func (rt *Router) handleMigrationPlan(w http.ResponseWriter, r *http.Request) {
	req, disc, ok := rt.decodeWizard(w, r)
	if !ok {
		return
	}
	_, rep, err := rt.analyzeWizard(r.Context(), req, disc)
	if err != nil {
		rt.internalError(w, "api: migration plan failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleMigrationImport handles POST /api/v1/migration/import: the selected
// items (only) with their overrides, or just the preview when dry_run is set.
func (rt *Router) handleMigrationImport(w http.ResponseWriter, r *http.Request) {
	req, disc, ok := rt.decodeWizard(w, r)
	if !ok {
		return
	}
	plan, rep, err := rt.analyzeWizard(r.Context(), req, disc)
	if err != nil {
		rt.internalError(w, "api: migration import failed", err)
		return
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, rep)
		return
	}
	if len(req.Only) == 0 {
		writeError(w, http.StatusBadRequest, "select at least one item in only, or use dry_run to preview")
		return
	}
	stripBindMounts(rt, r, plan)
	for _, a := range plan.Apps {
		if len(a.Secrets) > 0 && rt.secrets == nil {
			writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
			return
		}
	}
	applied := platformimport.Apply(r.Context(), plan, &importApplier{rt: rt})
	rt.nudgeReconciler()
	rt.seedChecklists(r.Context(), req.Platform, applied, r)
	rt.logger.Info("api: migration import applied", slog.String("platform", req.Platform), slog.Any("counts", applied.Counts))
	writeJSON(w, http.StatusOK, migrationplan.MergeApplied(rep, applied))
}

func (rt *Router) seedChecklists(ctx context.Context, platform string, applied platformimport.Report, r *http.Request) {
	_, actorID, actorName, _ := rt.currentActor(r)
	actor := firstNonEmpty(actorName, actorID)
	now := time.Now().UTC()
	for _, it := range applied.Items {
		if it.Kind != "app" || it.Status != platformimport.StatusCreated {
			continue
		}
		c := migrationplan.New()
		_ = c.Complete(migrationplan.StepImported, actor, "", []string{"created from " + platform}, now)
		err := rt.migrationChecklists.SaveMigrationChecklist(ctx, store.MigrationChecklist{AppName: it.Target, Platform: platform,
			SourceID: it.SourceID, StepsJSON: c.Marshal(), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			rt.logger.Warn("api: migration checklist seed failed", slog.String("error", err.Error()), slog.String("app", it.Target))
		}
	}
}

type checklistStepView struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	At       string   `json:"at,omitempty"`
	Actor    string   `json:"actor,omitempty"`
	Note     string   `json:"note,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type checklistVolumeView struct {
	Name          string `json:"name"`
	SourceCommand string `json:"source_command,omitempty"`
	TargetCommand string `json:"target_command"`
}

type checklistView struct {
	App      string                `json:"app"`
	Platform string                `json:"platform,omitempty"`
	SourceID string                `json:"source_id,omitempty"`
	Steps    []checklistStepView   `json:"steps"`
	Current  string                `json:"current"`
	Progress int                   `json:"progress"`
	Total    int                   `json:"total"`
	Domains  []string              `json:"domains,omitempty"`
	Volumes  []checklistVolumeView `json:"volumes,omitempty"`
	// Tracked is false for an imported app that has not had a step recorded yet.
	Tracked bool `json:"tracked"`
}

func buildChecklistView(svc store.DesiredService, row *store.MigrationChecklist, c migrationplan.Checklist) checklistView {
	v := checklistView{App: svc.Name, Current: c.Current(), Progress: c.Progress(), Total: len(migrationplan.Steps), Domains: svc.Domains, Tracked: row != nil}
	if row != nil {
		v.Platform, v.SourceID = row.Platform, row.SourceID
	}
	if v.Platform == "" {
		v.Platform, v.SourceID, _ = strings.Cut(svc.Labels[platformimport.LabelSourceID], ":")
	}
	for _, id := range migrationplan.Steps {
		s := c.States[id]
		v.Steps = append(v.Steps, checklistStepView{ID: id, Status: s.Status, At: s.At, Actor: s.Actor, Note: s.Note, Evidence: s.Evidence})
	}
	prefix := "app-" + svc.Name + "-"
	for _, vol := range svc.Volumes {
		src := strings.TrimPrefix(vol.Name, prefix)
		v.Volumes = append(v.Volumes, checklistVolumeView{Name: vol.Name,
			SourceCommand: migrationplan.StatCommand("/var/lib/docker/volumes/" + src + "/_data"),
			TargetCommand: migrationplan.VolumeStatCommand(vol.Name)})
	}
	for _, b := range svc.BindMounts {
		v.Volumes = append(v.Volumes, checklistVolumeView{Name: b.HostPath,
			SourceCommand: migrationplan.StatCommand(b.HostPath), TargetCommand: migrationplan.StatCommand(b.HostPath)})
	}
	return v
}

// importedChecklistApps returns the readable imported apps, which are the
// ones a checklist can exist for.
func (rt *Router) importedChecklistApps(r *http.Request) ([]store.DesiredService, error) {
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err == nil {
		svcs, err = rt.authorizeReadableServices(r, svcs)
	}
	if err != nil {
		return nil, err
	}
	var out []store.DesiredService
	for _, s := range svcs {
		if s.Labels[platformimport.LabelSourceID] != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func (rt *Router) loadChecklist(ctx context.Context, svc store.DesiredService) (migrationplan.Checklist, *store.MigrationChecklist, error) {
	row, err := rt.migrationChecklists.GetMigrationChecklist(ctx, svc.Name)
	if errors.Is(err, store.ErrMigrationChecklistNotFound) {
		c := migrationplan.New()
		_ = c.Complete(migrationplan.StepImported, "", "", []string{"app was imported earlier"}, time.Now())
		return c, nil, nil
	}
	if err != nil {
		return migrationplan.Checklist{}, nil, err
	}
	c, err := migrationplan.Parse(row.StepsJSON)
	return c, &row, err
}

type checklistListResponse struct {
	Checklists []checklistView `json:"checklists"`
}

// handleListMigrationChecklists handles GET /api/v1/migration/checklists.
func (rt *Router) handleListMigrationChecklists(w http.ResponseWriter, r *http.Request) {
	apps, err := rt.importedChecklistApps(r)
	if err != nil {
		rt.internalError(w, "api: migration checklists: list apps failed", err)
		return
	}
	out := checklistListResponse{Checklists: []checklistView{}}
	for _, s := range apps {
		c, row, err := rt.loadChecklist(r.Context(), s)
		if err != nil {
			rt.internalError(w, "api: migration checklists: load failed", err, slog.String("app", s.Name))
			return
		}
		out.Checklists = append(out.Checklists, buildChecklistView(s, row, c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) checklistApp(w http.ResponseWriter, r *http.Request) (store.DesiredService, bool) {
	apps, err := rt.importedChecklistApps(r)
	if err != nil {
		rt.internalError(w, "api: migration checklist: list apps failed", err)
		return store.DesiredService{}, false
	}
	name := r.PathValue("app")
	for _, s := range apps {
		if s.Name == name {
			return s, true
		}
	}
	writeError(w, http.StatusNotFound, "no imported app named "+name)
	return store.DesiredService{}, false
}

// handleGetMigrationChecklist handles GET /api/v1/migration/checklists/{app}.
func (rt *Router) handleGetMigrationChecklist(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.checklistApp(w, r)
	if !ok {
		return
	}
	c, row, err := rt.loadChecklist(r.Context(), svc)
	if err != nil {
		rt.internalError(w, "api: migration checklist: load failed", err, slog.String("app", svc.Name))
		return
	}
	writeJSON(w, http.StatusOK, buildChecklistView(svc, row, c))
}

type checklistStepRequest struct {
	Action    string                     `json:"action"`
	Note      string                     `json:"note,omitempty"`
	Confirm   bool                       `json:"confirm,omitempty"`
	Databases []string                   `json:"databases,omitempty"`
	Volumes   []migrationplan.VolumeStat `json:"volumes,omitempty"`
}

type checklistStepError struct {
	Error    string   `json:"error"`
	Problems []string `json:"problems,omitempty"`
}

func writeStepBlocked(w http.ResponseWriter, msg string, problems []string) {
	writeJSON(w, http.StatusConflict, checklistStepError{Error: msg, Problems: problems})
}

// handleMigrationChecklistStep handles POST
// /api/v1/migration/checklists/{app}/steps/{step}: complete (after the
// step's own check passes) or reset it and the steps after it.
func (rt *Router) handleMigrationChecklistStep(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.checklistApp(w, r)
	if !ok {
		return
	}
	var req checklistStepRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWizardBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	step := r.PathValue("step")
	c, row, err := rt.loadChecklist(r.Context(), svc)
	if err != nil {
		rt.internalError(w, "api: migration checklist: load failed", err, slog.String("app", svc.Name))
		return
	}
	_, actorID, actorName, _ := rt.currentActor(r)
	actor := firstNonEmpty(actorName, actorID)
	now := time.Now().UTC()
	before := c.States[step]
	switch req.Action {
	case "reset":
		if err := c.Reset(step); err != nil {
			writeStepError(w, err)
			return
		}
	case "complete":
		evidence, problems := rt.checkStep(r, svc, step, req)
		if len(problems) > 0 {
			writeStepBlocked(w, "the "+step+" step cannot be completed yet", problems)
			return
		}
		if err := c.Complete(step, actor, req.Note, evidence, now); err != nil {
			writeStepError(w, err)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "action must be complete or reset")
		return
	}
	saved, err := rt.persistChecklist(r.Context(), svc, row, c, now)
	if err != nil {
		rt.internalError(w, "api: migration checklist: save failed", err, slog.String("app", svc.Name))
		return
	}
	rt.auditChecklistReopen(r, svc.Name, step, req.Action, before)
	writeJSON(w, http.StatusOK, buildChecklistView(svc, &saved, c))
}

// persistChecklist stores c for svc, keeping the platform and source id already recorded.
func (rt *Router) persistChecklist(ctx context.Context, svc store.DesiredService, row *store.MigrationChecklist, c migrationplan.Checklist, now time.Time) (store.MigrationChecklist, error) {
	saved := store.MigrationChecklist{AppName: svc.Name, StepsJSON: c.Marshal(), CreatedAt: now, UpdatedAt: now}
	if row != nil {
		saved.Platform, saved.SourceID, saved.CreatedAt = row.Platform, row.SourceID, row.CreatedAt
	} else {
		saved.Platform, saved.SourceID, _ = strings.Cut(svc.Labels[platformimport.LabelSourceID], ":")
	}
	if err := rt.migrationChecklists.SaveMigrationChecklist(ctx, saved); err != nil {
		return store.MigrationChecklist{}, fmt.Errorf("save migration checklist: %w", err)
	}
	return saved, nil
}

func writeStepError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, migrationplan.ErrUnknownStep):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeStepBlocked(w, err.Error(), nil)
	}
}

// checkStep runs the evidence check that gates a step and returns what it
// found, or the reasons it cannot be completed.
func (rt *Router) checkStep(r *http.Request, svc store.DesiredService, step string, req checklistStepRequest) (evidence, problems []string) {
	ctx := r.Context()
	switch step {
	case migrationplan.StepImported:
		return []string{"app exists here"}, nil
	case migrationplan.StepDataVerified:
		dbs := make([]migrationplan.DatabaseResult, 0, len(req.Databases))
		for _, name := range req.Databases {
			imp, found, err := rt.dataImports.GetDatabaseDataImport(ctx, name)
			if err != nil {
				return nil, []string{"could not read the data copy of " + name}
			}
			dbs = append(dbs, migrationplan.DatabaseResult{Name: name, Status: imp.Status, Checked: imp.Checked, Mismatched: imp.Mismatched, Found: found})
		}
		var want []string
		for _, v := range svc.Volumes {
			want = append(want, v.Name)
		}
		for _, b := range svc.BindMounts {
			want = append(want, b.HostPath)
		}
		return migrationplan.VerifyData(dbs, want, req.Volumes)
	case migrationplan.StepShadowRunning:
		ready, detail := rt.appReadiness(ctx, svc)
		if !ready {
			return nil, []string{"the app is not healthy here yet: " + detail}
		}
		return []string{"healthy here: " + detail}, nil
	case migrationplan.StepDNSSwitched:
		return rt.checkDNSSwitched(r, svc, req.Confirm)
	case migrationplan.StepOldStopped:
		if !req.Confirm {
			return nil, []string{"confirm that the old instance is stopped: the control plane cannot see it"}
		}
		return []string{"operator confirmed the old instance is stopped"}, nil
	}
	return nil, []string{"unknown step " + step}
}

func (rt *Router) checkDNSSwitched(r *http.Request, svc store.DesiredService, confirm bool) (evidence, problems []string) {
	if len(svc.Domains) == 0 {
		return []string{"the app has no domains to switch"}, nil
	}
	ctx := r.Context()
	targets, bad := rt.migrationTargetIPs(ctx, r)
	if bad != "" {
		return nil, []string{bad}
	}
	doms, err := rt.importedDomains(r, svc.Name)
	if err != nil {
		return nil, []string{"could not read the app's domains"}
	}
	prober := datamigrate.Prober{Resolver: rt.migrationResolver()}
	for _, d := range doms {
		ok := true
		for _, c := range prober.PostSwitch(ctx, d.domain, d.healthPath, targets) {
			if c.Status == datamigrate.CheckFail {
				ok = false
				problems = append(problems, d.domain+": "+c.ID+": "+c.Detail)
			}
		}
		if ok {
			evidence = append(evidence, d.domain+" resolves here and answers")
		}
	}
	if len(problems) > 0 && confirm {
		return append(evidence, "operator confirmed despite: "+strings.Join(problems, "; ")), nil
	}
	return evidence, problems
}
