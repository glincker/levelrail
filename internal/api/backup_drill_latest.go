package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// drillLatestRequest picks what to drill: an app volume, the newest backup on a target,
// or the control plane snapshot.
type drillLatestRequest struct {
	App          string `json:"app,omitempty"`
	Volume       string `json:"volume,omitempty"`
	TargetID     string `json:"target_id,omitempty"`
	ControlPlane bool   `json:"control_plane,omitempty"`
}

// drillLatestResponse is the one shape every kind answers with. Volume and target drills run
// in the background (status running, poll GET /api/v1/backups/drills/{id}); a control plane
// drill finishes inside the request.
type drillLatestResponse struct {
	Kind       string       `json:"kind"`
	Status     string       `json:"status"`
	DrillID    string       `json:"drill_id,omitempty"`
	BackupID   string       `json:"backup_id,omitempty"`
	DurationMS int64        `json:"duration_ms,omitempty"`
	Checks     []drillCheck `json:"checks,omitempty"`
	Error      string       `json:"error,omitempty"`
}

type drillCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// drillControlPlaneBusy keeps a second control plane drill from running alongside one in progress.
var drillControlPlaneBusy atomic.Bool

// authorizeDrillBackup applies the per resource write:sensitive decision to the
// app or database a backup belongs to, answering 404 when it is hidden.
func (rt *Router) authorizeDrillBackup(w http.ResponseWriter, r *http.Request, h store.BackupHistory) bool {
	return rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusNotFound, "backup not found"), drillBackupRef(h))
}

func drillBackupRef(h store.BackupHistory) resourceRef {
	if h.ResourceKind != store.BackupResourceKindVolume {
		return databaseRef(h.DatabaseName)
	}
	return appRef(h.ServiceName)
}

// writeDrillStartError maps a refused drill start to its status; it reports whether it wrote one.
func (rt *Router) writeDrillStartError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, backup.ErrDrillBusy):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, backup.ErrDrillCapacity):
		writeError(w, http.StatusTooManyRequests, err.Error())
	default:
		return false
	}
	return true
}

// handleBackupDrillLatest handles POST /api/v1/backups/drill: restore drill of the latest backup.
func (rt *Router) handleBackupDrillLatest(w http.ResponseWriter, r *http.Request) {
	var req drillLatestRequest
	if !decodeReq(w, r, &req) {
		return
	}
	chosen := 0
	for _, set := range []bool{req.App != "", req.TargetID != "", req.ControlPlane} {
		if set {
			chosen++
		}
	}
	if chosen != 1 {
		writeError(w, http.StatusBadRequest, "set exactly one of app, target_id or control_plane")
		return
	}
	if req.ControlPlane {
		_, _, abilities, err := rt.callerPrincipal(r)
		if err != nil || !hasAbility(abilities, AbilityRoot) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if !drillControlPlaneBusy.CompareAndSwap(false, true) {
			writeError(w, http.StatusConflict, "a drill for this resource is already running")
			return
		}
		defer drillControlPlaneBusy.Store(false)
		rt.drillControlPlane(w, r)
		return
	}
	if req.App != "" && !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusNotFound, "app not found"), appRef(req.App)) {
		return
	}
	historyID, h, ok := rt.latestDrillBackup(w, r, req)
	if !ok {
		return
	}
	if !rt.authorizeDrillBackup(w, r, h) {
		return
	}
	if rt.backupProtection == nil {
		writeError(w, http.StatusNotImplemented, "restore drills are not configured on this control plane")
		return
	}
	id, err := rt.backupProtection.StartDrill(r.Context(), historyID)
	if rt.writeDrillStartError(w, err) {
		return
	}
	if err != nil {
		rt.internalError(w, "api: start latest drill failed", err, slog.String("backup_id", historyID))
		return
	}
	writeJSON(w, http.StatusAccepted, drillLatestResponse{Kind: "backup", Status: "running", DrillID: id, BackupID: historyID})
}

// latestDrillBackup finds the newest succeeded, drillable backup for the request; it writes the error itself.
func (rt *Router) latestDrillBackup(w http.ResponseWriter, r *http.Request, req drillLatestRequest) (string, store.BackupHistory, bool) {
	var rows []store.BackupHistory
	var err error
	if req.App != "" {
		svc, gerr := rt.apps.GetDesiredService(r.Context(), req.App)
		if errors.Is(gerr, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return "", store.BackupHistory{}, false
		}
		if gerr != nil {
			rt.internalError(w, "api: drill: load app failed", gerr, slog.String("name", req.App))
			return "", store.BackupHistory{}, false
		}
		vol := req.Volume
		if vol == "" {
			if len(svc.Volumes) != 1 {
				writeError(w, http.StatusBadRequest, "this app has no single volume, set volume")
				return "", store.BackupHistory{}, false
			}
			vol = svc.Volumes[0].Name
		}
		rows, err = rt.serviceVolumeBackupHistory.ListServiceVolumeBackupHistory(r.Context(), req.App, vol, 20, nil)
	} else {
		var all []store.BackupHistory
		all, err = rt.backupHistory.ListAllBackupHistory(r.Context(), 200, nil)
		for _, h := range all {
			if h.TargetID == req.TargetID {
				rows = append(rows, h)
			}
		}
	}
	if err != nil {
		rt.internalError(w, "api: drill: list backups failed", err)
		return "", store.BackupHistory{}, false
	}
	for _, h := range rows {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		// Rows the caller may not drill are skipped before any status body, so they stay invisible.
		denied, derr := rt.firstDeniedRef(r, AbilityWriteSensitive, []resourceRef{drillBackupRef(h)})
		if derr != nil {
			rt.internalError(w, "api: drill: authz failed", derr)
			return "", store.BackupHistory{}, false
		}
		if denied != nil {
			continue
		}
		if t, terr := rt.backupTargets.GetBackupTarget(r.Context(), h.TargetID); terr == nil && (strings.HasPrefix(t.Endpoint, store.LocalEndpointScheme) || strings.HasPrefix(t.Endpoint, backup.AgentEndpointScheme)) {
			writeError(w, http.StatusConflict, "restore drills are "+backup.DrillNotApplicable)
			return "", store.BackupHistory{}, false
		}
		return h.ID, h, true
	}
	writeError(w, http.StatusNotFound, "no succeeded backup to drill")
	return "", store.BackupHistory{}, false
}

// drillControlPlane verifies the newest control plane snapshot (checksum and database integrity)
// and answers pass or fail with the elapsed time.
func (rt *Router) drillControlPlane(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.cpBackupsOrNotImplemented(w)
	if !ok {
		return
	}
	list, err := m.List()
	if err != nil {
		rt.internalError(w, "api: drill: list control plane backups failed", err)
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "no control plane backup to drill")
		return
	}
	newest := list[0]
	for _, i := range list {
		if i.CreatedAt.After(newest.CreatedAt) {
			newest = i
		}
	}
	start := time.Now()
	res, err := m.Verify(r.Context(), newest.Name)
	out := drillLatestResponse{Kind: "control_plane", DurationMS: time.Since(start).Milliseconds()}
	switch {
	case err != nil:
		out.Status, out.Error = "fail", "verification could not run"
		rt.logger.Warn("api: control plane drill failed", slog.String("error", err.Error()))
	case res.OK:
		out.Status = "pass"
	default:
		out.Status = "fail"
	}
	for _, c := range res.Checks {
		out.Checks = append(out.Checks, drillCheck{Name: c.Name, OK: c.OK, Detail: neutralCheckDetail(c.OK)})
	}
	writeJSON(w, http.StatusOK, out)
}

// neutralCheckDetail keeps checksums, host paths and raw errors out of drill answers.
func neutralCheckDetail(ok bool) string {
	if ok {
		return "check passed"
	}
	return "check failed"
}
