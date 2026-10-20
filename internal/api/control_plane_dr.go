package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
)

// ControlPlaneDR is the surface the /system/control-plane-dr routes need.
type ControlPlaneDR interface {
	Status(ctx context.Context) (cpbackup.Status, error)
	UpdateConfig(ctx context.Context, u cpbackup.ConfigUpdate) error
	ListRemote(ctx context.Context) ([]cpbackup.Remote, error)
	StartBackup() error
	StartDrill() error
	BuildEscrow(ctx context.Context, in cpbackup.EscrowInput) (cpbackup.EscrowBundle, error)
	AckEscrow(ctx context.Context) error
}

// EscrowMaterialReader returns the master key and the other recovery files an
// escrow bundle protects. It is called only to build a bundle and its result
// is never logged.
type EscrowMaterialReader func() (cpbackup.EscrowMaterial, error)

// WithControlPlaneDR enables the /api/v1/system/control-plane-dr routes and
// the doctor's disaster recovery check. Without it the routes return 501.
func WithControlPlaneDR(dr ControlPlaneDR, material EscrowMaterialReader) Option {
	return func(rt *Router) {
		rt.cpDR = dr
		rt.cpDRMaterial = material
	}
}

// SetControlPlaneDR enables the disaster recovery routes after the router is
// built, for wiring that needs the router's own dependencies first.
func (rt *Router) SetControlPlaneDR(dr ControlPlaneDR, material EscrowMaterialReader) {
	rt.cpDR = dr
	rt.cpDRMaterial = material
}

func (rt *Router) cpDROrNotImplemented(w http.ResponseWriter) (ControlPlaneDR, bool) {
	if rt.cpDR == nil {
		writeError(w, http.StatusNotImplemented, "control plane disaster recovery is not configured")
		return nil, false
	}
	return rt.cpDR, true
}

func (rt *Router) writeCPDRError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, cpbackup.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, cpbackup.ErrNotConfigured):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, cpbackup.ErrBusy):
		writeError(w, http.StatusConflict, "a run is already in progress")
	case errors.Is(err, cpbackup.ErrEscrowSameBucket):
		writeError(w, http.StatusConflict, err.Error())
	default:
		rt.logger.Error("api: control plane disaster recovery failed", slog.String("action", action), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (rt *Router) handleGetControlPlaneDR(w http.ResponseWriter, r *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	st, err := dr.Status(r.Context())
	if err != nil {
		rt.writeCPDRError(w, "status", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type controlPlaneDRSettingsRequest struct {
	Enabled        bool     `json:"enabled"`
	TargetID       string   `json:"target_id"`
	Recipients     []string `json:"recipients"`
	Schedule       string   `json:"schedule"`
	DrillSchedule  string   `json:"drill_schedule"`
	RetainDaily    int      `json:"retain_daily"`
	RetainWeekly   int      `json:"retain_weekly"`
	RetainMonthly  int      `json:"retain_monthly"`
	EscrowTargetID string   `json:"escrow_target_id"`
}

func (rt *Router) handleUpdateControlPlaneDR(w http.ResponseWriter, r *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	var req controlPlaneDRSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	err := dr.UpdateConfig(r.Context(), cpbackup.ConfigUpdate{
		Enabled: req.Enabled, TargetID: req.TargetID, Recipients: req.Recipients, Schedule: req.Schedule,
		DrillSchedule: req.DrillSchedule, EscrowTargetID: req.EscrowTargetID,
		Retention: cpbackup.Retention{Daily: req.RetainDaily, Weekly: req.RetainWeekly, Monthly: req.RetainMonthly},
	})
	if err != nil {
		rt.writeCPDRError(w, "update settings", err)
		return
	}
	rt.handleGetControlPlaneDR(w, r)
}

func (rt *Router) handleListControlPlaneDRBackups(w http.ResponseWriter, r *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	list, err := dr.ListRemote(r.Context())
	if err != nil {
		rt.writeCPDRError(w, "list", err)
		return
	}
	if list == nil {
		list = []cpbackup.Remote{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (rt *Router) handleRunControlPlaneDRBackup(w http.ResponseWriter, _ *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	if err := dr.StartBackup(); err != nil {
		rt.writeCPDRError(w, "run backup", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

func (rt *Router) handleRunControlPlaneDRDrill(w http.ResponseWriter, _ *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	if err := dr.StartDrill(); err != nil {
		rt.writeCPDRError(w, "run drill", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

type controlPlaneDREscrowRequest struct {
	Recipients []string `json:"recipients"`
	Upload     bool     `json:"upload"`
}

// handleControlPlaneDREscrow returns an age-encrypted escrow bundle. The master
// key is encrypted server-side to public recipients, so no plaintext key ever
// crosses the wire or reaches a log.
func (rt *Router) handleControlPlaneDREscrow(w http.ResponseWriter, r *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	if rt.cpDRMaterial == nil {
		writeError(w, http.StatusNotImplemented, "no master key is available to escrow")
		return
	}
	var req controlPlaneDREscrowRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	material, err := rt.cpDRMaterial()
	if err != nil {
		rt.logger.Error("api: read master key for escrow failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "the master key could not be read")
		return
	}
	bundle, err := dr.BuildEscrow(r.Context(), cpbackup.EscrowInput{EscrowMaterial: material, Recipients: req.Recipients, Upload: req.Upload})
	if err != nil {
		rt.writeCPDRError(w, "escrow", err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (rt *Router) handleAckControlPlaneDREscrow(w http.ResponseWriter, r *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	if err := dr.AckEscrow(r.Context()); err != nil {
		rt.logger.Warn("api: acknowledge escrow failed", slog.String("error", err.Error()))
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	rt.handleGetControlPlaneDR(w, r)
}

func (rt *Router) doctorCheckControlPlaneDR(ctx context.Context) doctorCheckResource {
	const code, name = "control_plane_dr", "Control plane disaster recovery"
	if rt.cpDR == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "off-box disaster recovery is not available on this instance"}
	}
	st, err := rt.cpDR.Status(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "could not read disaster recovery status"}
	}
	fix := rt.cliName() + " control-plane-backups schedule show"
	docs := "/disaster-recovery"
	if !st.Enabled {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message: "off-box encrypted backups are not enabled, so losing this machine loses the control plane",
			Fix:     fix, DocsPath: docs,
		}
	}
	if len(st.Warnings) > 0 {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn, Message: st.Warnings[0].Message,
			Fix: fix, DocsPath: docs,
		}
	}
	msg := "encrypted off-box backups are running and the last restore drill passed"
	if st.LastBackupAt != nil {
		msg += " (newest backup " + time.Since(*st.LastBackupAt).Round(time.Minute).String() + " ago)"
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
}
