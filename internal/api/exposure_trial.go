package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applyExposureOnTrial installs the rule without storing it, so it vanishes on
// its own after the confirm window or when the control plane restarts.
func (rt *Router) applyExposureOnTrial(w http.ResponseWriter, r *http.Request, want exposure.Restriction, plan exposure.Plan, lockout []string) {
	var previous *exposure.Restriction
	if rows, err := rt.exposureStore.ListExposureRestrictions(r.Context()); err == nil {
		for _, row := range rows {
			if row.Port == want.Port && row.Protocol == want.Protocol {
				previous = &exposure.Restriction{Port: row.Port, Protocol: row.Protocol, Allow: row.Allow}
			}
		}
	}
	window := rt.runtimeSettings.FirewallConfirmWindow(r.Context())
	ch, err := rt.exposure.ApplyWithRevert(r.Context(), want, previous, window)
	if err != nil {
		rt.logger.Error("api: exposure trial apply failed", slog.Int("port", want.Port), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not apply the rule: "+err.Error())
		return
	}
	rt.auditExposure(r, "exposure_restrict_trial", want.Port, want.Protocol, want.Allow)
	writeJSON(w, http.StatusOK, exposurePlanResource{
		Plan: plan, Port: want.Port, Protocol: want.Protocol, Allow: want.Allow, Warnings: rt.exposureWarnings(r, want.Allow),
		Applied: true, Lockout: lockout, Pending: true, RevertAt: ch.ExpiresAt.Format(time.RFC3339),
	})
}

// handleExposureConfirm handles POST /api/v1/firewall/exposure/restrictions/{protocol}/{port}/confirm:
// it keeps a rule that is on trial and stores it.
func (rt *Router) handleExposureConfirm(w http.ResponseWriter, r *http.Request) {
	_, port, proto, ok := rt.exposureTarget(w, r)
	if !ok {
		return
	}
	ch, found := rt.exposure.Confirm(proto, port)
	if !found {
		writeError(w, http.StatusNotFound, "no restriction is waiting for confirmation on this port; it may have been rolled back already")
		return
	}
	row := store.ExposureRestriction{Port: ch.Port, Protocol: ch.Protocol, Allow: ch.Allow, CreatedBy: rt.accessActor(r).name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.exposureStore.SaveExposureRestriction(r.Context(), row); err != nil {
		_ = rt.exposure.Remove(r.Context(), ch.Port, ch.Protocol)
		rt.internalError(w, "api: exposure confirm: save failed, rule rolled back", err, slog.Int("port", port))
		return
	}
	rt.auditExposure(r, "exposure_confirm", ch.Port, ch.Protocol, ch.Allow)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, exposurePlanResource{Port: ch.Port, Protocol: ch.Protocol, Allow: ch.Allow, Applied: true})
}

// auditExposure records who changed which port for which sources. It never
// stores request bodies or tokens.
func (rt *Router) auditExposure(r *http.Request, action string, port int, proto string, allow []string) {
	a := rt.accessActor(r)
	rt.saveExposureAudit(r, a.kind, a.id, a.name, clientIP(r), r.Method, action, port, proto, allow)
}

func (rt *Router) saveExposureAudit(r *http.Request, kind, id, name, remote, method, action string, port int, proto string, allow []string) {
	eid, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: exposure audit id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(map[string]any{"port": port, "protocol": proto, "sources": allow, "actor": name})
	entry := store.AuditEntry{
		ID: eid, ActorType: kind, ActorID: id, ActorName: name,
		Ability: AbilityRoot, Method: method, Path: "/api/v1/firewall/exposure", StatusCode: http.StatusOK,
		RemoteAddr: remote, CreatedAt: store.FormatAuditTime(time.Now()),
		Action: action, Detail: string(raw),
	}
	if r != nil {
		entry.ClientKind = clientKindFromUserAgent(r.Header.Get("User-Agent"))
	}
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: exposure audit save failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

// auditExposureRevert records a rollback done by the timer, which has no caller.
func (rt *Router) auditExposureRevert(ch exposure.Change, err error) {
	action := "exposure_revert"
	if err != nil {
		action = "exposure_revert_failed"
		rt.logger.Error("api: exposure revert failed", slog.Int("port", ch.Port), slog.String("error", err.Error()))
	}
	rt.saveExposureAudit(nil, "system", "exposure", "system", "", "", action, ch.Port, ch.Protocol, ch.Allow)
}
