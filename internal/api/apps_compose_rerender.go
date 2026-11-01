package api

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

// serviceEnvPatcher is the env-only write behind the domain re-render.
// *store.DB provides it.
type serviceEnvPatcher interface {
	PatchServiceEnv(ctx context.Context, name string, set map[string]string) error
}

// legacyURLKey matches env names that older stored revisions filled with a
// full URL through ${SERVICE_FQDN_*} (FQDN used to resolve to a URL).
var legacyURLKey = regexp.MustCompile(`(?i)(url|uri|urls|origin|origins|endpoint|url_root)$|^(domain_client|domain_server|woodpecker_host|uptime_app_client_host)$`)

// rerenderDomainEnv re-resolves SERVICE_FQDN and SERVICE_URL env values of
// the compose app owning serviceName after its domains changed, then
// recreates the members whose env moved. Best effort: a domain edit that
// already landed is never failed by it. Members the caller may not deploy
// are left untouched, since this path restarts them.
func (rt *Router) rerenderDomainEnv(r *http.Request, serviceName string) {
	ctx := r.Context()
	revs := rt.composeRevisionStore()
	patcher, ok := rt.apps.(serviceEnvPatcher)
	if revs == nil || rt.appGroups == nil || !ok {
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
	principal, err := rt.bulkPrincipal(r)
	if err != nil {
		rt.logger.Warn("api: rerender domain env: resolve caller failed", slog.String("error", err.Error()))
		return
	}
	updated, err := rerenderMemberEnv(rev, svc.AppID, members)
	if err != nil {
		rt.logger.Warn("api: rerender domain env failed", slog.String("app", svc.AppID), slog.String("error", err.Error()))
		return
	}
	for _, m := range updated {
		if !principal.allowed(AbilityDeploy, m.Name) {
			rt.logger.Info("api: rerender domain env: member skipped, caller lacks deploy", slog.String("service", m.Name))
			continue
		}
		keys := changedEnvKeys(members, m)
		if err := patcher.PatchServiceEnv(ctx, m.Name, pickEnv(m.Env, keys)); err != nil {
			rt.logger.Error("api: rerender domain env: save failed", slog.String("service", m.Name), slog.String("error", err.Error()))
			continue
		}
		rt.auditDomainRerender(r, m.Name, keys)
		if err := rt.apps.RestartService(ctx, m.Name); err != nil {
			rt.logger.Error("api: rerender domain env: recreate failed", slog.String("service", m.Name), slog.String("error", err.Error()))
		}
	}
}

// auditDomainRerender records env keys (names only, never values) rewritten
// and the restart that followed a domain edit.
func (rt *Router) auditDomainRerender(r *http.Request, member string, keys []string) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityDeploy, Method: "DOMAIN_ENV_RERENDER",
		Path:       "/api/v1/apps/" + member + "/domains#env-rerender:" + strings.Join(keys, ","),
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: domain rerender audit save failed", slog.String("error", err.Error()), slog.String("service", member))
	}
}

// changedEnvKeys lists the keys of updated whose value differs from the
// snapshot of the same member in before.
func changedEnvKeys(before []store.DesiredService, updated store.DesiredService) []string {
	var old map[string]string
	for _, b := range before {
		if b.Name == updated.Name {
			old = b.Env
		}
	}
	var keys []string
	for k, v := range updated.Env {
		if ov, ok := old[k]; !ok || ov != v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func pickEnv(env map[string]string, keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = env[k]
	}
	return out
}

// rerenderMemberEnv returns the members whose domain-derived env values
// differ once the stored compose body is resolved against their current
// domains. Only keys built purely from FQDN or URL tokens are touched, and
// nothing is generated or persisted, so manual env edits and stored
// secrets are left alone.
func rerenderMemberEnv(rev store.ComposeRevision, appID string, members []store.DesiredService) ([]store.DesiredService, error) {
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
	// The install-time x-levelrail-domains block is stale after an edit;
	// the members' current domains are the source of truth.
	file.Domains = nil
	file.RuntimeDomains = map[string]string{}
	for _, m := range members {
		if len(m.Domains) > 0 {
			file.RuntimeDomains[strings.TrimPrefix(m.Name, appID+"-")] = m.Domains[0]
		}
	}
	for svcKey, svc := range file.Services {
		for envKey, tmpl := range svc.Environment {
			if legacyURLKey.MatchString(envKey) && isWholeFQDNToken(tmpl) {
				svc.Environment[envKey] = strings.Replace(tmpl, "SERVICE_FQDN", "SERVICE_URL", 1)
			}
		}
		file.Services[svcKey] = svc
	}
	noGen := func(string, string, int) (string, error) { return "", nil }
	noPersist := func(string, string, string) error { return nil }
	if _, _, err := compose.ResolveMagicVars(file, noGen, noPersist); err != nil {
		return nil, err
	}

	var updated []store.DesiredService
	for _, m := range members {
		key := strings.TrimPrefix(m.Name, appID+"-")
		next := maps.Clone(m.Env)
		changed := false
		for envKey, tmpl := range raw.Services[key].Environment {
			if !onlyDomainVars(tmpl) {
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

func isWholeFQDNToken(tmpl string) bool {
	vars := compose.FindMagicVars(tmpl)
	return len(vars) == 1 && vars[0].Token == tmpl && vars[0].Kind == "FQDN"
}

// onlyDomainVars is true when tmpl has at least one FQDN or URL token and
// no other magic var, so a re-render cannot touch a generated secret.
func onlyDomainVars(tmpl string) bool {
	vars := compose.FindMagicVars(tmpl)
	for _, v := range vars {
		if v.Kind != "FQDN" && v.Kind != "URL" {
			return false
		}
	}
	return len(vars) > 0
}
