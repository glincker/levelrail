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
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appimport"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

type appImportVolumeResource struct {
	Name          string `json:"name"`
	SourceName    string `json:"source_name,omitempty"`
	ContainerPath string `json:"container_path"`
	Copied        bool   `json:"copied"`
	CopiedAt      string `json:"copied_at,omitempty"`
}

type appImportItemResource struct {
	SourceID  string                    `json:"source_id"`
	Name      string                    `json:"name"`
	Target    string                    `json:"target,omitempty"`
	Kind      string                    `json:"kind"`
	State     string                    `json:"state"`
	Selected  bool                      `json:"selected"`
	Reason    string                    `json:"reason,omitempty"`
	Entry     appimport.Entry           `json:"entry"`
	Domains   []string                  `json:"domains,omitempty"`
	Volumes   []appImportVolumeResource `json:"volumes,omitempty"`
	Rewritten []appimport.AppliedChange `json:"rewritten,omitempty"`
	Remaining []string                  `json:"remaining,omitempty"`
	AppPath   string                    `json:"app_path,omitempty"`
}

type appImportSessionResource struct {
	ID        string                     `json:"id"`
	Platform  string                     `json:"platform"`
	SourceURL string                     `json:"source_url"`
	Step      string                     `json:"step"`
	Collision string                     `json:"collision"`
	Mappings  []appimport.Mapping        `json:"mappings"`
	Suggested []appimport.Mapping        `json:"suggested_mappings"`
	Connected bool                       `json:"connected"`
	Running   bool                       `json:"running"`
	Items     []appImportItemResource    `json:"items"`
	Databases []appimport.DBRef          `json:"databases"`
	Preflight *appimport.PreflightResult `json:"preflight,omitempty"`
	Diff      []appimport.Change         `json:"diff,omitempty"`
	Counts    map[string]int             `json:"counts"`
	States    map[string]int             `json:"states"`
	CreatedAt string                     `json:"created_at"`
	UpdatedAt string                     `json:"updated_at"`
	Notes     []string                   `json:"notes,omitempty"`
	StepOrder []string                   `json:"step_order"`
}

func newAppImportID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return appImportIDPrefix + hex.EncodeToString(b), nil
}

func isUnsupportedEntry(e appimport.Entry) bool { return e.Verdict == appimport.VerdictUnsupported }

func defaultSelected(e appimport.Entry) bool {
	return e.Verdict == appimport.VerdictReady || e.Verdict == appimport.VerdictNotes
}

func volumeResources(vs []store.AppImportVolume) []appImportVolumeResource {
	out := make([]appImportVolumeResource, 0, len(vs))
	for _, v := range vs {
		out = append(out, appImportVolumeResource{Name: v.Name, SourceName: v.SourceName, ContainerPath: v.ContainerPath, Copied: v.Copied, CopiedAt: v.CopiedAt})
	}
	return out
}

// itemsFromInventory merges a fresh inventory into stored items. Existing
// items keep their state, selection and volume confirmations.
func itemsFromInventory(inv appimport.Inventory, existing []store.AppImportItem, rewritten map[string][]appimport.AppliedChange) []store.AppImportItem {
	byID := map[string]store.AppImportItem{}
	for _, it := range existing {
		byID[it.SourceID] = it
	}
	var out []store.AppImportItem
	for _, e := range inv.Entries() {
		it, had := byID[e.SourceID]
		if !had {
			it = store.AppImportItem{SourceID: e.SourceID, State: store.AppImportPlanned, Selected: defaultSelected(e)}
			for _, v := range e.Volumes {
				name := v.Name
				if name == "" {
					name = v.HostPath
				}
				it.Volumes = append(it.Volumes, store.AppImportVolume{Name: name, SourceName: name, ContainerPath: v.ContainerPath})
			}
			it.Domains = e.Domains
		}
		it.SourceName, it.Kind = e.Name, e.Kind
		raw, _ := json.Marshal(appImportRecord{Entry: e, Rewritten: rewritten[e.SourceID]})
		it.EntryJSON = string(raw)
		out = append(out, it)
	}
	return out
}

func (rt *Router) appImportView(_ context.Context, sess store.AppImportSession, items []store.AppImportItem, calc *appImportCalc) appImportSessionResource {
	view := appImportSessionResource{
		ID: sess.ID, Platform: sess.Platform, SourceURL: sess.SourceURL, Step: sess.Step, Collision: sess.Collision,
		Mappings: toMappings(sess.Mappings), Suggested: []appimport.Mapping{}, Items: []appImportItemResource{}, Databases: []appimport.DBRef{},
		Counts: map[string]int{}, States: map[string]int{}, StepOrder: appImportSteps,
		CreatedAt: sess.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: sess.UpdatedAt.UTC().Format(time.RFC3339),
		Connected: rt.appImportLive.get(sess.ID) != nil, Running: rt.appImportLive.isRunning(sess.ID),
	}
	seenDB := map[string]bool{}
	fresh := map[string]appimport.Entry{}
	if calc != nil {
		for _, e := range calc.inv.Entries() {
			fresh[e.SourceID] = e
		}
	}
	for _, it := range items {
		rec := decodeAppImportRecord(it.EntryJSON)
		if e, ok := fresh[it.SourceID]; ok {
			rec.Entry = e
			rec.Rewritten = calc.rewritten[it.SourceID]
		}
		res := appImportItemResource{
			SourceID: it.SourceID, Name: it.SourceName, Target: it.TargetName, Kind: it.Kind, State: it.State, Selected: it.Selected,
			Reason: it.Reason, Entry: rec.Entry, Domains: it.Domains, Volumes: volumeResources(it.Volumes), Rewritten: rec.Rewritten,
			Remaining: appimport.Remaining(rec.Entry, it),
		}
		if it.TargetName != "" && it.State != store.AppImportPlanned && it.State != store.AppImportRolledBack {
			res.AppPath = "/apps/" + it.TargetName
		}
		view.Items = append(view.Items, res)
		view.Counts[rec.Entry.Verdict]++
		view.States[it.State]++
		for _, d := range rec.Entry.Databases {
			if !seenDB[d.SourceID] {
				seenDB[d.SourceID] = true
				view.Databases = append(view.Databases, d)
			}
		}
	}
	sort.Slice(view.Items, func(i, j int) bool { return view.Items[i].Name < view.Items[j].Name })
	if calc != nil {
		view.Preflight = &calc.pre
		view.Diff = calc.pre.Diff
		view.Suggested = calc.suggested
		if len(calc.inv.Databases) > 0 {
			view.Databases = calc.inv.Databases
		}
	}
	return view
}

// loadAppImport loads a session and its items, writing the error response.
func (rt *Router) loadAppImport(w http.ResponseWriter, r *http.Request) (store.AppImportSession, []store.AppImportItem, bool) {
	id := r.PathValue("id")
	sess, err := rt.appImports.GetAppImportSession(r.Context(), id)
	if errors.Is(err, store.ErrAppImportSessionNotFound) {
		writeError(w, http.StatusNotFound, "import session not found")
		return sess, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: app import: load session failed", err, slog.String("session_id", id))
		return sess, nil, false
	}
	items, err := rt.appImports.ListAppImportItems(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: app import: load items failed", err, slog.String("session_id", id))
		return sess, nil, false
	}
	return sess, items, true
}

// currentView evaluates the session when its source is connected and falls
// back to the stored inventory when it is not.
func (rt *Router) currentView(ctx context.Context, sess store.AppImportSession, items []store.AppImportItem) (appImportSessionResource, error) {
	live := rt.appImportLive.get(sess.ID)
	if live == nil {
		return rt.appImportView(ctx, sess, items, nil), nil
	}
	calc, err := rt.calcAppImport(ctx, sess, items, live)
	if err != nil {
		return appImportSessionResource{}, err
	}
	return rt.appImportView(ctx, sess, items, calc), nil
}

func (rt *Router) writeAppImportView(w http.ResponseWriter, r *http.Request, status int, sess store.AppImportSession, items []store.AppImportItem) {
	view, err := rt.currentView(r.Context(), sess, items)
	if err != nil {
		rt.internalError(w, "api: app import: evaluate session failed", err, slog.String("session_id", sess.ID))
		return
	}
	writeJSON(w, status, view)
}

type appImportCreateRequest struct {
	platformImportRequest
	Mappings []appimport.Mapping `json:"mappings,omitempty"`
}

func (rt *Router) decodeAppImportCreate(w http.ResponseWriter, r *http.Request) (appImportCreateRequest, platformimport.Source, bool) {
	var req appImportCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, nil, false
	}
	for _, m := range req.Mappings {
		if err := appimport.ValidateMapping(m); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return req, nil, false
		}
	}
	src, ok := rt.openImportSource(w, req.platformImportRequest)
	return req, src, ok
}

func (rt *Router) discoverForImport(ctx context.Context, src platformimport.Source, token string) (*platformimport.Discovery, error) {
	disc, err := src.Discover(ctx)
	if err != nil {
		msg := err.Error()
		if token != "" {
			msg = strings.ReplaceAll(msg, token, "[redacted]")
		}
		rt.logger.Warn("api: app import: reading the source failed", slog.String("error", msg))
		return nil, fmt.Errorf("reading the source platform: %s", msg)
	}
	return disc, nil
}

// handleAppImportPlan handles POST /api/v1/migration/apps/plan: a stateless
// plan that stores nothing and creates nothing.
func (rt *Router) handleAppImportPlan(w http.ResponseWriter, r *http.Request) {
	req, src, ok := rt.decodeAppImportCreate(w, r)
	if !ok {
		return
	}
	disc, err := rt.discoverForImport(r.Context(), src, req.Token)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	sess := store.AppImportSession{ID: "plan", Platform: req.Platform, SourceURL: sourceBase(req.URL), Step: appImportStepInventory, Collision: collisionOrDefault(req.Collision)}
	for _, m := range req.Mappings {
		sess.Mappings = append(sess.Mappings, store.AppImportMapping{From: m.From, To: m.To})
	}
	ictx, _, err := rt.appImportContext(r.Context())
	if err != nil {
		rt.internalError(w, "api: app import: read targets failed", err)
		return
	}
	inv := appimport.BuildInventory(disc, ictx)
	items := itemsFromInventory(inv, nil, nil)
	if len(req.Only) > 0 {
		for i := range items {
			items[i].Selected = false
			for _, o := range req.Only {
				if o == items[i].SourceID || strings.EqualFold(o, items[i].SourceName) {
					items[i].Selected = true
				}
			}
		}
	}
	live := &appImportLive{req: req.platformImportRequest, disc: disc, expires: time.Now().Add(time.Minute)}
	calc, err := rt.calcAppImport(r.Context(), sess, items, live)
	if err != nil {
		rt.internalError(w, "api: app import: plan failed", err)
		return
	}
	view := rt.appImportView(r.Context(), sess, items, calc)
	view.Connected = false
	writeJSON(w, http.StatusOK, view)
}

func collisionOrDefault(c string) string {
	if c == "" {
		return platformimport.CollisionSuffix
	}
	return c
}

// handleCreateAppImportSession handles POST /api/v1/migration/apps/sessions.
// The token is kept in memory with a TTL and never stored.
func (rt *Router) handleCreateAppImportSession(w http.ResponseWriter, r *http.Request) {
	req, src, ok := rt.decodeAppImportCreate(w, r)
	if !ok {
		return
	}
	disc, err := rt.discoverForImport(r.Context(), src, req.Token)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	id, err := newAppImportID()
	if err != nil {
		rt.internalError(w, "api: app import: new session id failed", err)
		return
	}
	now := time.Now().UTC()
	sess := store.AppImportSession{ID: id, Platform: req.Platform, SourceURL: sourceBase(req.URL), Step: appImportStepInventory,
		Collision: collisionOrDefault(req.Collision), CreatedAt: now, UpdatedAt: now}
	for _, m := range req.Mappings {
		sess.Mappings = append(sess.Mappings, store.AppImportMapping{From: m.From, To: m.To})
	}
	ictx, _, err := rt.appImportContext(r.Context())
	if err != nil {
		rt.internalError(w, "api: app import: read targets failed", err)
		return
	}
	items := itemsFromInventory(appimport.BuildInventory(disc, ictx), nil, nil)
	if err := rt.appImports.CreateAppImportSession(r.Context(), sess, items); err != nil {
		rt.internalError(w, "api: app import: create session failed", err, slog.String("session_id", id))
		return
	}
	rt.appImportLive.set(id, req.platformImportRequest, disc)
	rt.logger.Info("api: app import session created", slog.String("session_id", id), slog.String("platform", req.Platform), slog.Int("items", len(items)))
	stored, err := rt.appImports.ListAppImportItems(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: app import: load items failed", err, slog.String("session_id", id))
		return
	}
	rt.writeAppImportView(w, r, http.StatusCreated, sess, stored)
}

// handleListAppImportSessions handles GET /api/v1/migration/apps/sessions.
func (rt *Router) handleListAppImportSessions(w http.ResponseWriter, r *http.Request) {
	list, err := rt.appImports.ListAppImportSessions(r.Context())
	if err != nil {
		rt.internalError(w, "api: app import: list sessions failed", err)
		return
	}
	out := make([]appImportSessionResource, 0, len(list))
	for _, s := range list {
		items, err := rt.appImports.ListAppImportItems(r.Context(), s.ID)
		if err != nil {
			rt.internalError(w, "api: app import: list items failed", err, slog.String("session_id", s.ID))
			return
		}
		v := rt.appImportView(r.Context(), s, items, nil)
		v.Items = nil
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// handleGetAppImportSession handles GET .../sessions/{id}.
func (rt *Router) handleGetAppImportSession(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	rt.writeAppImportView(w, r, http.StatusOK, sess, items)
}

// handleDeleteAppImportSession handles DELETE .../sessions/{id}. It forgets
// the session; apps it staged are kept (use rollback to remove them).
func (rt *Router) handleDeleteAppImportSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := rt.appImports.DeleteAppImportSession(r.Context(), id); err != nil {
		rt.internalError(w, "api: app import: delete session failed", err, slog.String("session_id", id))
		return
	}
	rt.appImportLive.forget(id)
	w.WriteHeader(http.StatusNoContent)
}

// handleConnectAppImportSession handles POST .../sessions/{id}/connect:
// re-attaches the source token after a restart or its expiry.
func (rt *Router) handleConnectAppImportSession(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	req, src, ok := rt.decodeAppImportCreate(w, r)
	if !ok {
		return
	}
	if sourceBase(req.URL) != sess.SourceURL || req.Platform != sess.Platform {
		writeError(w, http.StatusBadRequest, "the session was created for "+sess.SourceURL+", connect to the same source")
		return
	}
	disc, err := rt.discoverForImport(r.Context(), src, req.Token)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	rt.appImportLive.set(sess.ID, req.platformImportRequest, disc)
	ictx, _, err := rt.appImportContext(r.Context())
	if err != nil {
		rt.internalError(w, "api: app import: read targets failed", err)
		return
	}
	merged := itemsFromInventory(appimport.BuildInventory(disc, ictx), items, nil)
	for _, it := range merged {
		if err := rt.appImports.SaveAppImportItem(r.Context(), it, time.Now()); err != nil {
			rt.internalError(w, "api: app import: save item failed", err, slog.String("session_id", sess.ID))
			return
		}
	}
	rt.writeAppImportView(w, r, http.StatusOK, sess, merged)
}

type putAppImportPlanRequest struct {
	Mappings  *[]appimport.Mapping `json:"mappings,omitempty"`
	Selected  *[]string            `json:"selected,omitempty"`
	Collision string               `json:"collision,omitempty"`
	Step      string               `json:"step,omitempty"`
}

// handlePutAppImportPlan handles PUT .../sessions/{id}/plan: the mapping
// table, the selection and the collision mode. It writes nothing but the
// session; the response carries the fresh preflight and visible diff.
func (rt *Router) handlePutAppImportPlan(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	var req putAppImportPlanRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Mappings != nil {
		sess.Mappings = nil
		for _, m := range *req.Mappings {
			if err := appimport.ValidateMapping(m); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			sess.Mappings = append(sess.Mappings, store.AppImportMapping{From: m.From, To: m.To})
		}
	}
	if req.Collision != "" {
		if req.Collision != platformimport.CollisionSuffix && req.Collision != platformimport.CollisionSkip {
			writeError(w, http.StatusBadRequest, "collision must be suffix or skip")
			return
		}
		sess.Collision = req.Collision
	}
	if req.Step != "" {
		if !stringIn(appImportSteps, req.Step) {
			writeError(w, http.StatusBadRequest, "unknown step")
			return
		}
		sess.Step = req.Step
	}
	if req.Selected != nil {
		pick := map[string]bool{}
		for _, id := range *req.Selected {
			pick[id] = true
		}
		for i := range items {
			rec := decodeAppImportRecord(items[i].EntryJSON)
			want := pick[items[i].SourceID] && !isUnsupportedEntry(rec.Entry)
			if items[i].Selected != want {
				items[i].Selected = want
				if err := rt.appImports.SaveAppImportItem(r.Context(), items[i], time.Now()); err != nil {
					rt.internalError(w, "api: app import: save item failed", err, slog.String("session_id", sess.ID))
					return
				}
			}
		}
	}
	if err := rt.appImports.UpdateAppImportSession(r.Context(), sess, time.Now()); err != nil {
		rt.internalError(w, "api: app import: update session failed", err, slog.String("session_id", sess.ID))
		return
	}
	sess.UpdatedAt = time.Now().UTC()
	rt.writeAppImportView(w, r, http.StatusOK, sess, items)
}

func stringIn(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
