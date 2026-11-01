package api

import (
	"context"
	"log/slog"
	"maps"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

// rerenderDomainEnv re-resolves SERVICE_FQDN and SERVICE_URL env values of
// the compose app owning serviceName after its domains changed, then
// recreates the members whose env moved. Best effort: a domain edit that
// already landed is never failed by it.
func (rt *Router) rerenderDomainEnv(ctx context.Context, serviceName string) {
	revs := rt.composeRevisionStore()
	if revs == nil || rt.appGroups == nil || rt.appCompose == nil {
		return
	}
	svc, err := rt.apps.GetDesiredService(ctx, serviceName)
	if err != nil || svc.AppID == "" {
		return
	}
	rev, err := revs.GetComposeRevision(ctx, svc.AppID, 0)
	if err != nil {
		return
	}
	members, err := rt.appGroups.ListServicesByApp(ctx, svc.AppID)
	if err != nil {
		rt.logger.Warn("api: rerender domain env: list members failed", slog.String("app", svc.AppID), slog.String("error", err.Error()))
		return
	}
	updated, err := rerenderMemberEnv(rev, svc.AppID, members, rt.generateComposeSecret(ctx, svc.AppID), rt.persistComposeSecret(ctx, svc.AppID))
	if err != nil {
		rt.logger.Warn("api: rerender domain env failed", slog.String("app", svc.AppID), slog.String("error", err.Error()))
		return
	}
	for _, m := range updated {
		if err := rt.appCompose.SaveDesiredService(ctx, m); err != nil {
			rt.logger.Error("api: rerender domain env: save failed", slog.String("service", m.Name), slog.String("error", err.Error()))
			continue
		}
		if err := rt.apps.RestartService(ctx, m.Name); err != nil {
			rt.logger.Error("api: rerender domain env: recreate failed", slog.String("service", m.Name), slog.String("error", err.Error()))
		}
	}
}

// rerenderMemberEnv returns the members whose domain-derived env values
// differ once the stored compose body is resolved against their current
// domains. Only keys that reference a domain token are touched, so manual
// env edits and generated secrets are left alone.
func rerenderMemberEnv(
	rev store.ComposeRevision,
	appID string,
	members []store.DesiredService,
	generate func(kind, key string, length int) (string, error),
	persist func(serviceKey, envKey, value string) error,
) ([]store.DesiredService, error) {
	raw, err := compose.Parse([]byte(rev.Compose))
	if err != nil {
		return nil, err
	}
	file, err := compose.Parse([]byte(rev.Compose))
	if err != nil {
		return nil, err
	}
	if _, err := file.ApplyProfiles(rev.Profiles); err != nil {
		return nil, err
	}
	file.RuntimeDomains = map[string]string{}
	for _, m := range members {
		if len(m.Domains) > 0 {
			file.RuntimeDomains[strings.TrimPrefix(m.Name, appID+"-")] = m.Domains[0]
		}
	}
	if _, _, err := compose.ResolveMagicVars(file, generate, persist); err != nil {
		return nil, err
	}

	var updated []store.DesiredService
	for _, m := range members {
		key := strings.TrimPrefix(m.Name, appID+"-")
		next := maps.Clone(m.Env)
		changed := false
		for envKey, tmpl := range raw.Services[key].Environment {
			if !usesDomainVar(tmpl) {
				continue
			}
			val, ok := file.Services[key].Environment[envKey]
			if !ok || next[envKey] == val {
				continue
			}
			if next == nil {
				next = map[string]string{}
			}
			next[envKey] = val
			changed = true
		}
		if changed {
			m.Env = next
			updated = append(updated, m)
		}
	}
	return updated, nil
}

func usesDomainVar(tmpl string) bool {
	for _, v := range compose.FindMagicVars(tmpl) {
		if v.Kind == "FQDN" || v.Kind == "URL" {
			return true
		}
	}
	return false
}
