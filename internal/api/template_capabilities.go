package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionCapabilityGranted is written once per granted capability at install.
const AuditActionCapabilityGranted = "templates.capability_granted"

// capabilityPolicy reads the per-class runtime settings; a missing store means off.
func (rt *Router) capabilityPolicy(ctx context.Context) catalog.CapabilityPolicy {
	on := rt.runtimeSettings != nil && rt.runtimeSettings.Bool(ctx, runtimesettings.KeyTemplatesCapNetAdmin)
	return catalog.CapabilityPolicy{Enabled: map[string]bool{catalog.CapClassNetAdmin: on}}
}

// resolveTemplateCapabilities returns the grants an install may carry, or writes the refusal.
// Elevated needs AbilityRoot on top of the setting; the container side of an Elevated
// grant is not persisted yet, so it is refused rather than dropped silently.
func (rt *Router) resolveTemplateCapabilities(w http.ResponseWriter, r *http.Request, m *catalog.Manifest) ([]catalog.CapabilityGrant, bool) {
	grants, err := m.CapabilityGrants(rt.capabilityPolicy(r.Context()))
	if errors.Is(err, catalog.ErrCapabilityDisabled) {
		writeError(w, http.StatusForbidden, err.Error())
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return nil, false
	}
	for _, g := range grants {
		if g.Tier == catalog.CapTierReview {
			continue
		}
		if !rt.callerHasAbility(r, AbilityRoot) {
			writeError(w, http.StatusForbidden, "capability "+g.Class+" needs the root ability")
			return nil, false
		}
		writeError(w, http.StatusConflict, "capability "+g.Class+" is allowed by settings but the runtime does not carry it to containers yet")
		return nil, false
	}
	return grants, true
}

// templateCapGrants re-derives the cap_add grants for an app that came from a
// template, through the same checks as install. Revisions never store grants.
func (rt *Router) templateCapGrants(w http.ResponseWriter, r *http.Request, templateID string) (map[string][]string, bool) {
	if templateID == "" {
		return nil, true
	}
	tpl, ok := catalog.TemplateByID(templateID)
	if !ok || tpl.Manifest == nil {
		return nil, true
	}
	grants, ok := rt.resolveTemplateCapabilities(w, r, tpl.Manifest)
	if !ok {
		return nil, false
	}
	return catalog.CapGrantMap(grants), true
}

// auditCapabilityGrants writes one event per grant, after the deploy succeeded.
// It returns false when any event could not be written so the caller can say so.
func (rt *Router) auditCapabilityGrants(r *http.Request, app string, m *catalog.Manifest, grants []catalog.CapabilityGrant) bool {
	if rt.auditLog == nil {
		return true
	}
	clean := true
	a := rt.accessActor(r)
	digest := m.CapabilityDigest()
	for _, g := range grants {
		id, err := store.NewAuditEntryID()
		if err != nil {
			rt.logger.Warn("api: capability audit id failed", slog.String("error", err.Error()))
			clean = false
			continue
		}
		detail, _ := json.Marshal(map[string]any{"app": app, "template": m.ID, "class": g.Class, "tier": g.Tier, "service": g.Service, "caps": g.Caps, "digest": digest})
		entry := store.AuditEntry{
			ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
			Ability: AbilityDeploy, Method: http.MethodPost, Path: r.URL.Path, StatusCode: http.StatusOK,
			RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
			ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
			Action:     AuditActionCapabilityGranted, Detail: string(detail),
		}
		if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
			rt.logger.Warn("api: capability audit save failed", slog.String("error", err.Error()), slog.String("app", app))
			clean = false
		}
	}
	return clean
}
