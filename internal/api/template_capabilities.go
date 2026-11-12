package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionCapabilityGranted is written once per granted capability on install,
// and once per capability an upgrade, auto-rollback or rollback newly adds.
const AuditActionCapabilityGranted = "templates.capability_granted"

// capabilityPolicy reads the per-class runtime settings; a missing store means off.
func (rt *Router) capabilityPolicy(ctx context.Context) catalog.CapabilityPolicy {
	on := rt.runtimeSettings != nil && rt.runtimeSettings.Bool(ctx, runtimesettings.KeyTemplatesCapNetAdmin)
	return catalog.CapabilityPolicy{Enabled: map[string]bool{catalog.CapClassNetAdmin: on}}
}

// resolveTemplateCapabilities returns the grants an install or upgrade may carry, or writes the refusal.
// Elevated needs AbilityRoot on top of the setting, both checked at request time.
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
	}
	return grants, true
}

// templateCapGrants re-derives the cap_add grants for an upgrade of an app that came from a
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

// heldCaps maps each service key of the app to the grants it holds now.
func (rt *Router) heldCaps(ctx context.Context, app string) (map[string][]string, error) {
	members, err := rt.appGroups.ListServicesByApp(ctx, app)
	if err != nil {
		return nil, err
	}
	held := make(map[string][]string, len(members))
	for _, m := range members {
		held[strings.TrimPrefix(m.Name, app+"-")] = m.GrantedCaps
	}
	return held, nil
}

// addedCaps lists "service:CAP" for every cap in want that held lacks, sorted.
func addedCaps(held, want map[string][]string) []string {
	var added []string
	for svc, caps := range want {
		for _, c := range caps {
			if !slices.Contains(held[svc], c) {
				added = append(added, svc+":"+c)
			}
		}
	}
	sort.Strings(added)
	return added
}

// servicesCapMap reads the grants a set of translated services would carry, by service key.
func servicesCapMap(app string, services []store.DesiredService) map[string][]string {
	out := map[string][]string{}
	for _, s := range services {
		if len(s.GrantedCaps) > 0 {
			out[strings.TrimPrefix(s.Name, app+"-")] = s.GrantedCaps
		}
	}
	return out
}

// composeCapAdds reads the cap_add a compose body asks for, by service key, for the upgrade preview.
func composeCapAdds(body string) map[string][]string {
	f, err := compose.Parse([]byte(body))
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for key, svc := range f.Services {
		for _, c := range svc.HostAccess.CapAdd {
			out[key] = append(out[key], spec.NormalizeCap(c))
		}
	}
	return out
}

// normalizeAcceptedCaps canonicalizes "svc:cap" entries so they compare against addedCaps.
func normalizeAcceptedCaps(in []string) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		svc, c, ok := strings.Cut(strings.TrimSpace(e), ":")
		if !ok {
			out = append(out, e)
			continue
		}
		out = append(out, svc+":"+spec.NormalizeCap(c))
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// rollbackCapGrants resolves the grants a rollback may carry. Caps the app already holds
// pass without a root re-check; a new Elevated cap still needs root, and an Elevated class
// switched off is left out (the compose layer then drops it) so a flipped setting cannot strand a rollback.
func (rt *Router) rollbackCapGrants(w http.ResponseWriter, r *http.Request, app, templateID string) (map[string][]string, bool) {
	if templateID == "" {
		return nil, true
	}
	tpl, ok := catalog.TemplateByID(templateID)
	if !ok || tpl.Manifest == nil {
		return nil, true
	}
	held, err := rt.heldCaps(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: rollback: read held capabilities failed", err, slog.String("app", app))
		return nil, false
	}
	policy := rt.capabilityPolicy(r.Context())
	out := map[string][]string{}
	for _, g := range tpl.Manifest.DeclaredCapabilities() {
		if g.Tier != catalog.CapTierReview && (g.Tier != catalog.CapTierElevated || !policy.Enabled[g.Class]) {
			continue
		}
		for _, c := range g.Caps {
			if !slices.Contains(held[g.Service], c) && g.Tier != catalog.CapTierReview && !rt.callerHasAbility(r, AbilityRoot) {
				writeError(w, http.StatusForbidden, "capability "+g.Class+" needs the root ability")
				return nil, false
			}
			out[g.Service] = append(out[g.Service], c)
		}
	}
	return out, true
}

// auditAddedCapabilities writes templates.capability_granted for each cap a deploy newly
// gave an app, so an upgrade or rollback is as visible as the install that first granted it.
func (rt *Router) auditAddedCapabilities(r *http.Request, app, templateID, event string, added []string) {
	if rt.auditLog == nil || len(added) == 0 {
		return
	}
	digest := ""
	if tpl, ok := catalog.TemplateByID(templateID); ok && tpl.Manifest != nil {
		digest = tpl.Manifest.CapabilityDigest()
	}
	a := rt.accessActor(r)
	for _, e := range added {
		svc, capName, _ := strings.Cut(e, ":")
		tier := catalog.CapTierReview
		if slices.Contains(dockerguard.GrantableCaps, capName) {
			tier = catalog.CapTierElevated
		}
		id, err := store.NewAuditEntryID()
		if err != nil {
			rt.logger.Warn("api: capability audit id failed", slog.String("error", err.Error()))
			continue
		}
		detail, _ := json.Marshal(map[string]any{"app": app, "template": templateID, "service": svc, "caps": []string{capName}, "tier": tier, "event": event, "digest": digest})
		entry := store.AuditEntry{
			ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
			Ability: AbilityDeploy, Method: http.MethodPost, Path: r.URL.Path, StatusCode: http.StatusOK,
			RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
			ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
			Action:     AuditActionCapabilityGranted, Detail: string(detail),
		}
		if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
			rt.logger.Warn("api: capability audit save failed", slog.String("error", err.Error()), slog.String("app", app))
		}
	}
}
