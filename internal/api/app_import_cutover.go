package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appimport"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/store"
)

type appImportVolumeGuide struct {
	SourceID string `json:"source_id"`
	Index    int    `json:"index"`
	datamigrate.VolumeGuide
	Copied   bool   `json:"copied"`
	CopiedAt string `json:"copied_at,omitempty"`
}

type appImportVolumesResponse struct {
	Source  string                 `json:"source"`
	Guides  []appImportVolumeGuide `json:"guides"`
	Warning string                 `json:"warning"`
}

func findItem(items []store.AppImportItem, key string) int {
	for i, it := range items {
		if it.SourceID == key || it.TargetName == key {
			return i
		}
	}
	return -1
}

// handleAppImportVolumes handles GET .../volumes?source=user@host. It only
// prints commands: the control plane never opens SSH to the source.
func (rt *Router) handleAppImportVolumes(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	source := r.URL.Query().Get("source")
	if err := datamigrate.ValidateSSHTarget(source); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	guides := []appImportVolumeGuide{}
	for _, it := range items {
		if !it.Selected || it.TargetName == "" || stageable(it.State) {
			continue
		}
		for i, v := range it.Volumes {
			ref := datamigrate.VolumeRef{Name: v.Name, ContainerPath: v.ContainerPath, SourceName: v.SourceName}
			if strings.HasPrefix(v.Name, "/") {
				ref = datamigrate.VolumeRef{HostPath: v.Name, ContainerPath: v.ContainerPath}
			}
			for _, g := range datamigrate.GuideVolumes(it.TargetName, "app-"+it.TargetName+"-", source, []datamigrate.VolumeRef{ref}) {
				guides = append(guides, appImportVolumeGuide{SourceID: it.SourceID, Index: i, VolumeGuide: g, Copied: v.Copied, CopiedAt: v.CopiedAt})
			}
		}
	}
	rt.setAppImportStep(r.Context(), sess, appImportStepVolumes)
	writeJSON(w, http.StatusOK, appImportVolumesResponse{Source: source, Guides: guides,
		Warning: "Run these as root on this node, once while the source is live to move the bulk, then again right before the DNS switch to catch the last changes. They only read from the source."})
}

type setAppImportVolumeRequest struct {
	Copied bool `json:"copied"`
}

// handleSetAppImportVolume handles PUT .../items/{item}/volumes/{vol}: the
// operator's confirmation that a volume was copied.
func (rt *Router) handleSetAppImportVolume(w http.ResponseWriter, r *http.Request) {
	_, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	var req setAppImportVolumeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	idx := findItem(items, r.PathValue("item"))
	vol, err := strconv.Atoi(r.PathValue("vol"))
	if idx < 0 || err != nil || vol < 0 || vol >= len(items[idx].Volumes) {
		writeError(w, http.StatusNotFound, "volume not found")
		return
	}
	v := &items[idx].Volumes[vol]
	v.Copied = req.Copied
	v.CopiedAt = ""
	if req.Copied {
		v.CopiedAt = time.Now().UTC().Format(time.RFC3339)
	}
	rt.saveAppImportItem(r.Context(), items[idx])
	writeJSON(w, http.StatusOK, volumeResources(items[idx].Volumes))
}

type appImportCutoverItem struct {
	SourceID       string                   `json:"source_id"`
	Name           string                   `json:"name"`
	Target         string                   `json:"target"`
	State          string                   `json:"state"`
	Verdict        string                   `json:"verdict"`
	Routed         bool                     `json:"routed"`
	VolumesPending int                      `json:"volumes_pending"`
	Domains        []datamigrate.DomainPlan `json:"domains"`
}

type appImportCutoverResponse struct {
	Phase     string                 `json:"phase"`
	TargetIPs []string               `json:"target_ips"`
	Verdict   string                 `json:"verdict"`
	Items     []appImportCutoverItem `json:"items"`
	Guidance  []string               `json:"guidance"`
}

func appImportCutoverGuidance(haveIP bool) []string {
	g := []string{
		"Nothing on the source platform is touched by any step here. Keep it running until you are satisfied with the new copy.",
		"Lower each domain's TTL to 300 seconds first and wait out the old TTL, so the switch takes effect within minutes and can be reverted as fast.",
		"Run the final volume copy, then enable routing here: that starts the app and attaches its domains. Certificates are issued once DNS points at this node.",
		"Change one domain's DNS record at a time, then run the post-switch check (resolves here, certificate issued, health returns 200).",
		"To roll back, point the record at the source again. The source was never stopped.",
	}
	if !haveIP {
		g = append(g, "This node's public IP could not be detected. Pass the IP to check against explicitly.")
	}
	return g
}

func (rt *Router) cutoverItems(items []store.AppImportItem) []store.AppImportItem {
	var out []store.AppImportItem
	for _, it := range items {
		if it.Selected && len(it.Domains) > 0 && (it.State == store.AppImportVerified || it.State == store.AppImportRouted) {
			out = append(out, it)
		}
	}
	return out
}

func volumesPending(it store.AppImportItem) int {
	n := 0
	for _, v := range it.Volumes {
		if !v.Copied {
			n++
		}
	}
	return n
}

// handleAppImportCutover handles GET .../cutover. Read-only: it reads DNS
// and app status and changes nothing.
func (rt *Router) handleAppImportCutover(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	targets, bad := rt.migrationTargetIPs(r.Context(), r)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	resolver := rt.migrationResolver()
	cut := rt.cutoverItems(items)
	resp := appImportCutoverResponse{Phase: "pre-switch", TargetIPs: targets, Items: []appImportCutoverItem{}, Guidance: appImportCutoverGuidance(len(targets) > 0)}
	var all []datamigrate.DomainPlan
	for _, it := range cut {
		ready, detail := true, "verified here, stopped until routing is enabled"
		if it.State == store.AppImportRouted {
			if svc, err := rt.apps.GetDesiredService(r.Context(), it.TargetName); err == nil {
				ready, detail = rt.appReadiness(r.Context(), *svc)
			} else {
				ready, detail = false, "the app is missing"
			}
		}
		ci := appImportCutoverItem{SourceID: it.SourceID, Name: it.SourceName, Target: it.TargetName, State: it.State,
			Routed: it.State == store.AppImportRouted, VolumesPending: volumesPending(it), Domains: make([]datamigrate.DomainPlan, len(it.Domains))}
		rt.parallel(len(it.Domains), func(i int) {
			d := it.Domains[i]
			ci.Domains[i] = datamigrate.Evaluate(datamigrate.EvalInput{App: it.TargetName, Domain: d, Ready: ready, ReadyDetail: detail,
				DNS: resolver.Lookup(r.Context(), d), TargetIPs: targets})
		})
		ci.Verdict = datamigrate.Overall(ci.Domains)
		if ci.VolumesPending > 0 && ci.Verdict == datamigrate.VerdictGo {
			ci.Verdict = datamigrate.VerdictWait
		}
		all = append(all, ci.Domains...)
		resp.Items = append(resp.Items, ci)
	}
	resp.Verdict = datamigrate.Overall(all)
	rt.setAppImportStep(r.Context(), sess, appImportStepCutover)
	writeJSON(w, http.StatusOK, resp)
}

// handleAppImportCutoverVerify handles GET .../cutover/verify: the
// post-switch check for every routed app.
func (rt *Router) handleAppImportCutoverVerify(w http.ResponseWriter, r *http.Request) {
	_, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	targets, bad := rt.migrationTargetIPs(r.Context(), r)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	prober := datamigrate.Prober{Resolver: rt.migrationResolver()}
	resp := appImportCutoverResponse{Phase: "post-switch", TargetIPs: targets, Items: []appImportCutoverItem{},
		Guidance: []string{"Keep the source running until every domain shows switched here, then stop it only after you are happy with a full day of traffic."}}
	var all []datamigrate.DomainPlan
	for _, it := range rt.cutoverItems(items) {
		if it.State != store.AppImportRouted {
			continue
		}
		path := "/"
		if rec := decodeAppImportRecord(it.EntryJSON); rec.Entry.Health != nil && rec.Entry.Health.Path != "" {
			path = rec.Entry.Health.Path
		}
		ci := appImportCutoverItem{SourceID: it.SourceID, Name: it.SourceName, Target: it.TargetName, State: it.State, Routed: true, Domains: make([]datamigrate.DomainPlan, len(it.Domains))}
		rt.parallel(len(it.Domains), func(i int) {
			d := it.Domains[i]
			checks := prober.PostSwitch(r.Context(), d, path, targets)
			verdict := datamigrate.VerdictSwitch
			for _, c := range checks {
				if c.Status == datamigrate.CheckFail {
					verdict = datamigrate.VerdictNoGo
				}
			}
			ci.Domains[i] = datamigrate.DomainPlan{App: it.TargetName, Domain: d, Verdict: verdict, Checks: checks}
		})
		ci.Verdict = datamigrate.Overall(ci.Domains)
		all = append(all, ci.Domains...)
		resp.Items = append(resp.Items, ci)
	}
	resp.Verdict = datamigrate.Overall(all)
	writeJSON(w, http.StatusOK, resp)
}

type routeAppImportRequest struct {
	Enable bool `json:"enable"`
	// IgnoreVolumes lets routing proceed while volume copies are unconfirmed.
	IgnoreVolumes bool `json:"ignore_volumes,omitempty"`
}

// handleRouteAppImport handles POST .../items/{item}/route: the explicit
// cutover action that starts the staged app and attaches its domains, or
// detaches them again. DNS is never changed here.
func (rt *Router) handleRouteAppImport(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	var req routeAppImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	idx := findItem(items, r.PathValue("item"))
	if idx < 0 {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	it := &items[idx]
	editor, ok := rt.apps.(serviceDomainsEditor)
	if !ok {
		writeError(w, http.StatusNotImplemented, "domain editing is not available on this control plane")
		return
	}
	var err error
	if req.Enable {
		err = rt.enableAppImportRouting(r.Context(), editor, it, req)
	} else {
		err = rt.disableAppImportRouting(r.Context(), editor, it)
	}
	if err != nil {
		rt.logger.Warn("api: app import: routing change failed", slog.String("error", err.Error()), slog.String("session_id", sess.ID), slog.String("app", it.TargetName))
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	rt.saveAppImportItem(r.Context(), *it)
	rt.nudgeReconciler()
	rt.writeAppImportView(w, r, http.StatusOK, sess, items)
}

func (rt *Router) enableAppImportRouting(ctx context.Context, editor serviceDomainsEditor, it *store.AppImportItem, req routeAppImportRequest) error {
	if it.State != store.AppImportVerified && it.State != store.AppImportRouted {
		return fmt.Errorf("verify the app first, it is %s", it.State)
	}
	if n := volumesPending(*it); n > 0 && !req.IgnoreVolumes {
		return fmt.Errorf("%d volume(s) are not confirmed as copied: copy them and confirm, or route anyway", n)
	}
	if len(it.Domains) == 0 {
		return errors.New("the source app had no domains to attach")
	}
	_, _, err := editor.EditServiceDomains(ctx, it.TargetName, func([]string) ([]string, error) { return slices.Clone(it.Domains), nil })
	var taken *store.ErrDomainTaken
	switch {
	case errors.As(err, &taken):
		return fmt.Errorf("a domain is already attached to another app: %s", taken.Error())
	case err != nil:
		return fmt.Errorf("attach domains: %w", err)
	}
	if err := rt.apps.UpdateServiceSuspended(ctx, it.TargetName, false); err != nil {
		return fmt.Errorf("start the app: %w", err)
	}
	it.State, it.Reason = store.AppImportRouted, "routing enabled, change DNS to switch traffic"
	return nil
}

func (rt *Router) disableAppImportRouting(ctx context.Context, editor serviceDomainsEditor, it *store.AppImportItem) error {
	if it.State != store.AppImportRouted {
		return errors.New("routing is not enabled for this app")
	}
	if _, _, err := editor.EditServiceDomains(ctx, it.TargetName, func([]string) ([]string, error) { return nil, nil }); err != nil {
		return fmt.Errorf("detach domains: %w", err)
	}
	if err := rt.apps.UpdateServiceSuspended(ctx, it.TargetName, true); err != nil {
		return fmt.Errorf("stop the app: %w", err)
	}
	it.State, it.Reason = store.AppImportVerified, "routing disabled, the app is stopped again"
	return nil
}

// handleAppImportReceipt handles GET .../receipt: a downloadable record of
// the import with no secret values.
func (rt *Router) handleAppImportReceipt(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	var in []appimport.ReceiptInput
	for _, it := range items {
		if !it.Selected && it.State == store.AppImportPlanned {
			continue
		}
		rec := decodeAppImportRecord(it.EntryJSON)
		in = append(in, appimport.ReceiptInput{Item: it, Entry: rec.Entry, Rewritten: rec.Rewritten})
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="app-import-receipt-%s.json"`, sess.ID))
	writeJSON(w, http.StatusOK, appimport.BuildReceipt(sess, in, time.Now()))
}
