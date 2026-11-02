package api

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

// serviceEnvPatcher is the env-only compare-and-set write behind the domain
// re-render. *store.DB provides it.
type serviceEnvPatcher interface {
	PatchServiceEnv(ctx context.Context, name string, expect, set map[string]string) ([]string, error)
}

// domainEnvReport lists members whose env could not be re-rendered because
// the caller may not deploy them. Names are limited to what the caller may
// read; the rest are only counted.
type domainEnvReport struct {
	Skipped       []string
	SkippedHidden int
}

// envPatch is one member's pending re-render: Expect holds the stored value
// each key must still have, Set the new one.
type envPatch struct {
	Name        string
	Expect, Set map[string]string
	Kept        []string
}

// legacyURLKey matches env names that older stored revisions filled with a
// full URL through ${SERVICE_FQDN_*} (FQDN used to resolve to a URL).
var legacyURLKey = regexp.MustCompile(`(?i)(url|uri|urls|origin|origins|endpoint|url_root)$|^(domain_client|domain_server|woodpecker_host|uptime_app_client_host)$`)

// rerenderDomainEnv re-resolves SERVICE_FQDN and SERVICE_URL env values of
// the compose app owning serviceName after its domains changed from before,
// then recreates the members whose env moved. Best effort: a domain edit
// that already landed is never failed by it. Members the caller may not
// deploy are left untouched and reported, since this path restarts them.
func (rt *Router) rerenderDomainEnv(r *http.Request, serviceName string, before []string) domainEnvReport {
	var report domainEnvReport
	ctx := r.Context()
	revs := rt.composeRevisionStore()
	patcher, ok := rt.apps.(serviceEnvPatcher)
	if revs == nil || rt.appGroups == nil || !ok {
		return report
	}
	svc, err := rt.apps.GetDesiredService(ctx, serviceName)
	if err != nil || svc.AppID == "" {
		return report
	}
	rev, err := revs.GetComposeRevision(ctx, svc.AppID, 0)
	if err != nil {
		return report
	}
	members, err := rt.appGroups.ListServicesByApp(ctx, svc.AppID)
	if err != nil {
		rt.logger.Warn("api: rerender domain env: list members failed", slog.String("app", svc.AppID), slog.String("error", err.Error()))
		return report
	}
	principal, err := rt.bulkPrincipal(r)
	if err != nil {
		rt.logger.Warn("api: rerender domain env: resolve caller failed", slog.String("error", err.Error()))
		return report
	}
	patches, err := rerenderMemberEnv(rev, svc.AppID, members, serviceName, before)
	if err != nil {
		rt.logger.Warn("api: rerender domain env failed", slog.String("app", svc.AppID), slog.String("error", err.Error()))
		return report
	}
	for _, p := range patches {
		for _, k := range p.Kept {
			rt.logger.Info("api: rerender domain env: key edited by hand, left as is", slog.String("service", p.Name), slog.String("key", k))
		}
		if len(p.Set) == 0 {
			continue
		}
		if !principal.allowed(AbilityDeploy, p.Name) {
			rt.logger.Info("api: rerender domain env: member skipped, caller lacks deploy", slog.String("service", p.Name))
			if principal.allowed(AbilityRead, p.Name) {
				report.Skipped = append(report.Skipped, p.Name)
			} else {
				report.SkippedHidden++
			}
			continue
		}
		applied, err := patcher.PatchServiceEnv(ctx, p.Name, p.Expect, p.Set)
		if err != nil {
			rt.logger.Error("api: rerender domain env: save failed", slog.String("service", p.Name), slog.String("error", err.Error()))
			continue
		}
		if len(applied) == 0 {
			continue
		}
		rt.auditDomainRerender(r, p.Name, applied)
		if err := rt.apps.RestartService(ctx, p.Name); err != nil {
			rt.logger.Error("api: rerender domain env: recreate failed", slog.String("service", p.Name), slog.String("error", err.Error()))
		}
	}
	return report
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

// rerenderMemberEnv returns, per member, the domain-derived env values that
// move once the stored compose body is resolved against the current domains.
// A key is patched only if its stored value still equals what the previous
// domains (edited member had before) rendered, so hand edits are kept, and
// only if it resolves fully. Only keys built purely from FQDN or URL tokens
// are touched and nothing is generated or persisted.
func rerenderMemberEnv(rev store.ComposeRevision, appID string, members []store.DesiredService, edited string, before []string) ([]envPatch, error) {
	raw, err := compose.Parse([]byte(rev.Compose))
	if err != nil {
		return nil, err
	}
	domains := func(prev bool) map[string]string {
		out := map[string]string{}
		for _, m := range members {
			ds := m.Domains
			if prev && m.Name == edited {
				ds = before
			}
			if len(ds) > 0 {
				out[strings.TrimPrefix(m.Name, appID+"-")] = ds[0]
			}
		}
		return out
	}
	cur, curUnres, err := renderDomainEnv(rev, domains(false))
	if err != nil {
		return nil, err
	}
	old, _, err := renderDomainEnv(rev, domains(true))
	if err != nil {
		return nil, err
	}

	var out []envPatch
	for _, m := range members {
		key := strings.TrimPrefix(m.Name, appID+"-")
		p := envPatch{Name: m.Name, Expect: map[string]string{}, Set: map[string]string{}}
		for envKey, tmpl := range raw.Services[key].Environment {
			if !onlyDomainVars(tmpl) || curUnres[key+"\x00"+envKey] {
				continue
			}
			val, ok := cur[key][envKey]
			if !ok || m.Env[envKey] == val {
				continue
			}
			prev, prevOK := old[key][envKey]
			stored, has := m.Env[envKey]
			if !has {
				continue
			}
			if !prevOK || stored != prev {
				p.Kept = append(p.Kept, envKey)
				continue
			}
			p.Expect[envKey] = stored
			p.Set[envKey] = val
		}
		if len(p.Set)+len(p.Kept) > 0 {
			sort.Strings(p.Kept)
			out = append(out, p)
		}
	}
	return out, nil
}

// renderDomainEnv resolves the stored body against the given per-service
// domains and returns each service's env plus the (service, key) pairs left
// unresolved.
func renderDomainEnv(rev store.ComposeRevision, runtime map[string]string) (map[string]compose.Environment, map[string]bool, error) {
	file, err := compose.Parse([]byte(rev.Compose))
	if err != nil {
		return nil, nil, err
	}
	if _, err := file.ApplyProfiles(rev.Profiles); err != nil {
		return nil, nil, err
	}
	// The install-time x-levelrail-domains block is stale after an edit;
	// the members' domains are the source of truth.
	file.Domains = nil
	file.RuntimeDomains = runtime
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
	_, unresolved, err := compose.ResolveMagicVars(file, noGen, noPersist)
	if err != nil {
		return nil, nil, err
	}
	unres := map[string]bool{}
	for _, u := range unresolved {
		unres[u.Service+"\x00"+u.EnvKey] = true
	}
	envs := make(map[string]compose.Environment, len(file.Services))
	for k, svc := range file.Services {
		envs[k] = svc.Environment
	}
	return envs, unres, nil
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
