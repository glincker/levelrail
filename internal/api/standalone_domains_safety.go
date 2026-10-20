package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StandaloneSafety is what the API needs to refuse dangerous standalone targets.
type StandaloneSafety struct {
	// ControlPorts are this platform's own listeners on this server.
	ControlPorts []standalone.ControlPort
	// IngressPorts are the ports the embedded ingress listens on.
	IngressPorts []int
}

// WithStandaloneSafety sets the control and ingress ports standalone
// domain targets are checked against.
func WithStandaloneSafety(s StandaloneSafety) Option {
	return func(rt *Router) { rt.standaloneSafety = s }
}

func normalizeStandaloneDomain(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}

func validateStandaloneDomainName(domain string) string {
	if domain == "" {
		return "domain is required"
	}
	if err := ingress.ValidateWildcardDomain(domain); err != nil {
		return "invalid domain: " + err.Error()
	}
	if !standalone.ValidHostname(strings.TrimPrefix(domain, "*.")) || !strings.Contains(domain, ".") {
		return fmt.Sprintf("invalid domain %q: use a full hostname such as tools.example.com", domain)
	}
	return ""
}

func standaloneConflictMessage(domain, owner string) string {
	return fmt.Sprintf("domain %q is already used by %s; remove it there first", domain, owner)
}

func describeOwner(owner string) string {
	if owner == store.StandaloneDomainOwner {
		return "another standalone domain"
	}
	return fmt.Sprintf("app %q", owner)
}

// standaloneDomainOwner names whatever already serves domain, "" when free.
func (rt *Router) standaloneDomainOwner(ctx context.Context, domain string) string {
	if rows, err := rt.domains.ListServiceDomains(ctx); err == nil {
		for _, d := range rows {
			if strings.EqualFold(d.Domain, domain) {
				return fmt.Sprintf("app %q", d.ServiceName)
			}
		}
	}
	if sites, err := rt.staticSites.ListStaticSites(ctx); err == nil {
		for _, s := range sites {
			for _, d := range s.Domains {
				if strings.EqualFold(d, domain) {
					return fmt.Sprintf("static site %q", s.Name)
				}
			}
		}
	}
	if st := rt.standaloneStore(); st != nil {
		if _, err := st.GetStandaloneDomain(ctx, domain); err == nil {
			return "another standalone domain"
		}
	}
	if svcs, err := rt.apps.ListDesiredServices(ctx); err == nil {
		res := rt.fallbackResolver(ctx)
		for _, svc := range svcs {
			if d, ok := res.Domain(svc.Name); ok && strings.EqualFold(d, domain) {
				return fmt.Sprintf("app %q (its automatic hostname)", svc.Name)
			}
		}
	}
	return rt.platformDomainOwner(ctx, domain)
}

func (rt *Router) platformDomainOwner(ctx context.Context, domain string) string {
	if s, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil && strings.EqualFold(s.PrimaryDomain, domain) {
		return "the dashboard (primary domain)"
	}
	if rt.registry != nil {
		if s, err := rt.registry.GetRegistrySettings(ctx); err == nil && s.Enabled && strings.EqualFold(s.Host, domain) {
			return "the built-in registry"
		}
	}
	if rt.statusPage != nil {
		if s, err := rt.statusPage.GetStatusPageSettings(ctx); err == nil && s.Enabled && strings.EqualFold(s.CustomDomain, domain) {
			return "the status page"
		}
	}
	if rt.models != nil {
		if views, err := rt.models.List(ctx); err == nil {
			for _, v := range views {
				host := v.Model.Domain
				if host == "" {
					if u, err := url.Parse(v.BaseURL); err == nil {
						host = u.Hostname()
					}
				}
				if host != "" && strings.EqualFold(host, domain) {
					return fmt.Sprintf("AI model %q", v.Model.Name)
				}
			}
		}
	}
	return ""
}

func (rt *Router) checkStandaloneConflict(w http.ResponseWriter, r *http.Request, domain string) bool {
	if owner := rt.standaloneDomainOwner(r.Context(), domain); owner != "" {
		writeError(w, http.StatusConflict, standaloneConflictMessage(domain, owner))
		return false
	}
	return true
}

// assessStandalone normalizes d's target and options in place. msg is the
// client-facing error, empty when the target is acceptable.
func (rt *Router) assessStandalone(r *http.Request, d *store.StandaloneDomain) (a standalone.Assessment, display, msg string) {
	ctx := r.Context()
	t, err := standalone.ParseTarget(d.Target)
	if err != nil {
		return a, "", strings.TrimPrefix(err.Error(), "standalone: invalid target: ")
	}
	opts, err := standalone.Options{
		UpstreamTLS: d.UpstreamTLS, TLSSkipVerify: d.TLSSkipVerify, TLSServerName: d.TLSServerName,
		HostHeader: d.HostHeader, WebSocket: d.WebSocket, DialTimeoutSeconds: d.DialTimeoutSeconds,
		ReadTimeoutSeconds: d.ReadTimeoutSeconds, StripPrefix: d.StripPrefix,
	}.Validate(t)
	if err != nil {
		return a, "", strings.TrimPrefix(err.Error(), "standalone: invalid target: ")
	}
	d.Target, d.TargetKind = t.String(), t.Kind
	d.UpstreamTLS, d.TLSSkipVerify, d.TLSServerName = opts.UpstreamTLS, opts.TLSSkipVerify, opts.TLSServerName
	d.HostHeader, d.DialTimeoutSeconds, d.ReadTimeoutSeconds, d.StripPrefix = opts.HostHeader, opts.DialTimeoutSeconds, opts.ReadTimeoutSeconds, opts.StripPrefix
	display = t.Display(opts.UpstreamTLS)
	if t.Kind == standalone.KindApp {
		denied, err := rt.firstDeniedRef(r, AbilityWrite, []resourceRef{appRef(t.App)})
		if err != nil {
			rt.logger.Warn("api: standalone target app authz failed", slog.String("app", t.App), slog.String("error", err.Error()))
		}
		if denied != nil || err != nil {
			return a, display, fmt.Sprintf("app %q does not exist", t.App)
		}
		if _, err := rt.apps.GetDesiredService(ctx, t.App); err != nil {
			if errors.Is(err, store.ErrServiceNotFound) {
				return a, display, fmt.Sprintf("app %q does not exist", t.App)
			}
			rt.logger.Warn("api: standalone target app lookup failed", slog.String("app", t.App), slog.String("error", err.Error()))
		}
	}
	a, err = standalone.Assess(ctx, t, standalone.Env{
		Resolver:     rt.resolverOrDefault(),
		ControlPorts: rt.standaloneSafety.ControlPorts,
		OwnAddresses: rt.serverPublicIPs(ctx),
		IngressPorts: rt.standaloneSafety.IngressPorts,
	})
	if err != nil {
		return a, display, strings.TrimPrefix(err.Error(), "standalone: ")
	}
	return a, display, ""
}

// callerDisplayName is the audit name of whoever made r, "" when unknown.
func (rt *Router) callerDisplayName(r *http.Request) string {
	pt, id, _, err := rt.callerPrincipal(r)
	if err != nil || id == "" {
		return ""
	}
	return rt.auditActorName(r.Context(), auditActorType(pt), id, "")
}

func auditActorType(principalType string) string {
	if principalType == store.PrincipalTypeToken {
		return "token"
	}
	return auditActorSession
}

// recordStandaloneAudit writes a domain-scoped row so the domain's activity
// timeline shows who changed the proxy target.
func (rt *Router) recordStandaloneAudit(r *http.Request, action, domain string) {
	pt, actorID, _, err := rt.callerPrincipal(r)
	if err != nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: standalone audit id", slog.String("domain", domain), slog.String("error", err.Error()))
		return
	}
	at := auditActorType(pt)
	entry := store.AuditEntry{
		ID: id, ActorType: at, ActorID: actorID, ActorName: rt.auditActorName(r.Context(), at, actorID, ""),
		Ability: AbilityWriteSensitive, Method: r.Method, Path: "/api/v1/apps/" + standalone.Scope + "/domains/" + domain + "/target",
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action,
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: save standalone audit entry", slog.String("domain", domain), slog.String("error", err.Error()))
	}
}
