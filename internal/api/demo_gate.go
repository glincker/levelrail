package api

import (
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	demoCodeReadonly = "demo_readonly"
	demoCodeDisabled = "demo_disabled"
	demoCodeTokens   = "demo_tokens_disabled" //nolint:gosec // error code string, not a credential

	demoReadonlyMessage = "This is a read-only demo. Install it on your own server to try this for real."
	demoDisabledMessage = "This feature is not available in the demo."
)

// demoWriteAllowlist is the complete set of non-GET routes the demo serves.
// Every one is a sign-in step or a pure computation; anything else is denied.
var demoWriteAllowlist = []string{
	"POST /api/v1/demo/session",
	"POST /api/v1/demo/cta",
	"OPTIONS /api/v1/sentry/api/{project}/envelope/",
	"OPTIONS /api/v1/sentry/api/{project}/store/",
	"OPTIONS /api/v1/forms/{app}/{name}",
	"POST /api/v1/auth/login",
	"POST /api/v1/auth/logout",
	"POST /api/v1/apps/{name}/validate-spec",
	"POST /api/v1/pipelines/validate",
	"POST /api/v1/iam/policies/validate",
	"POST /api/v1/iam/preview",
	"POST /api/v1/iam/policy-templates/{id}/render",
}

// demoReadDenylist is the set of GET routes that would reach a host, a third
// party, or credential material. Read elevation makes root-only GETs visible
// to the visitor, so each of these is closed explicitly.
var demoReadDenylist = []string{
	// Shell, PTY and host provisioning.
	"/api/v1/apps/{name}/terminal",
	"/api/v1/apps/{name}/exec-access",
	"/api/v1/ssh-node-provisions",
	"/api/v1/ssh-node-provisions/{id}",
	"/api/v1/node-provisions",
	"/api/v1/node-provisions/{id}",
	"/api/v1/node-providers",
	"/api/v1/node-providers/{provider}/regions",
	"/api/v1/node-providers/{provider}/sizes",
	"/api/v1/system/cli-console/commands",
	"/api/v1/system/cli-console/session",
	"/api/v1/firewall/host",
	// Bulk exports and downloads.
	"/api/v1/apps/{name}/env/export",
	"/api/v1/export",
	"/api/v1/export/destinations",
	"/api/v1/export/destinations/{id}",
	"/api/v1/dns/zones/{zone}/records/export",
	"/api/v1/apps/{name}/loadbalancer/export",
	"/api/v1/audit-log/export",
	"/api/v1/system/backups",
	"/api/v1/system/backups/{name}/download",
	"/api/v1/archives/{id}/download",
	"/api/v1/system/control-plane-dr/backups",
	"/api/v1/databases/{name}/backups/{historyId}/download",
	"/api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download",
	"/api/v1/apps/{name}/logs/download",
	"/api/v1/apps/{name}/deploys/{deployId}/logs/download",
	"/api/v1/log-archive/objects",
	"/api/v1/log-archive/objects/download",
	// Third-party connect and callback flows.
	"/api/v1/github-app",
	"/api/v1/github-app/callback",
	"/api/v1/github-app/installations",
	"/api/v1/github-app/installed",
	"/api/v1/github-app/register/preview",
	"/api/v1/github-app/register/start",
	"/api/v1/github-app/repos",
	"/api/v1/github-app/repos/{owner}/{repo}/branches",
	"/api/v1/git-connections",
	"/api/v1/git-connections/{id}",
	"/api/v1/git-connections/{id}/branches",
	"/api/v1/gitlab-app/callback",
	"/api/v1/gitlab-app/connect",
	"/api/v1/gitea-app/callback",
	"/api/v1/gitea-app/connect",
	"/api/v1/bitbucket-app/callback",
	"/api/v1/bitbucket-app/connect",
	"/api/v1/apps/{name}/git-source/commits",
	"/api/v1/auth/oauth/{provider}/start",
	"/api/v1/auth/oauth/{provider}/link/start",
	"/api/v1/auth/oauth/{provider}/callback",
	"/api/v1/auth/session-links/{token}/consume",
	"/api/v1/team-invites/{token}",
	// Live probes that dial the host or the internet.
	"/api/v1/system/server-address",
	"/api/v1/updates/releases",
	"/api/v1/updates/preflight",
	"/api/v1/updates/history",
	"/api/v1/updates/schedule",
	"/api/v1/updates/settings",
	"/api/v1/updates/auto",
	"/api/v1/updates/rollback-plan",
	"/api/v1/updates/restore-points",
	"/api/v1/updates/self-upgrade/attempts",
	"/api/v1/updates/self-upgrade/plan",
	"/api/v1/settings/ingress/check",
	"/api/v1/ingress/connectivity",
	"/api/v1/dns/check",
	"/api/v1/dns/zones/{zone}/discover",
	"/api/v1/apps/{name}/domains/{domain}/check",
	"/api/v1/apps/{name}/domains/{domain}/search-visibility",
	"/api/v1/apps/{name}/domains/{domain}/go-live",
	"/api/v1/apps/{name}/domains/{domain}/dns-records",
	"/api/v1/dockerhub/search",
	"/api/v1/dockerhub/repositories/{namespace}/{repo}/tags",
	"/api/v1/registry/repositories",
	"/api/v1/registry/tags",
	"/api/v1/registry-credentials/{id}/repositories",
	"/api/v1/registry-credentials/{id}/tags",
	"/api/v1/domains/standalone/candidates",
	"/api/v1/external-databases/candidates",
	"/api/v1/external-databases/{name}/password",
	"/api/v1/mesh/connectivity",
	"/api/v1/storage/providers",
	"/api/v1/storage/orphans",
	// AI chat: costs money and takes free text.
	"/api/v1/ai/sessions",
	"/api/v1/ai/sessions/{id}",
	// Per-visitor credential and session metadata with nothing to show.
	"/api/v1/auth/tokens",
	"/api/v1/auth/trusted-devices",
	"/api/v1/auth/passkeys",
	"/api/v1/auth/2fa",
	"/api/v1/auth/device/requests",
	"/api/v1/auth/sign-in-requests",
	"/api/v1/auth/step-up",
	"/api/v1/security/sessions",
	"/api/v1/security/account",
	"/api/v1/settings/push-subscriptions",
	"/api/v1/settings/push-subscriptions/vapid-public-key",
}

// demoExtraDeniedPaths are denied for GET regardless of the denylist: the
// metrics endpoint is mounted outside /api/v1 and exposes host process metrics.
var demoExtraDeniedPaths = []string{"/metrics"}

type demoCtxKey struct{}

// demoMatchPath matches a request path against a pattern whose {name}
// segments match exactly one non-empty segment.
func demoMatchPath(pattern, p string) bool {
	ps := strings.Split(strings.Trim(pattern, "/"), "/")
	rs := strings.Split(strings.Trim(p, "/"), "/")
	if len(ps) != len(rs) {
		return false
	}
	for i, seg := range ps {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if rs[i] == "" {
				return false
			}
			continue
		}
		if seg != rs[i] {
			return false
		}
	}
	return true
}

func demoWriteAllowed(method, p string) bool {
	for _, entry := range demoWriteAllowlist {
		m, pat, _ := strings.Cut(entry, " ")
		if m == method && demoMatchPath(pat, p) {
			return true
		}
	}
	return false
}

func demoReadDenied(p string) bool {
	for _, pat := range demoReadDenylist {
		if demoMatchPath(pat, p) {
			return true
		}
	}
	for _, extra := range demoExtraDeniedPaths {
		if p == extra {
			return true
		}
	}
	return false
}

// demoGate is the outermost middleware in demo mode: default deny for every
// non-GET method, a denylist for risky GETs, and noindex on every response.
func (rt *Router) demoGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		p := path.Clean("/" + r.URL.Path)

		if rt.authEngine != nil && (p == rt.authEngine.Prefix() || strings.HasPrefix(p, rt.authEngine.Prefix()+"/")) {
			writeDemoError(w, http.StatusForbidden, demoCodeDisabled, demoDisabledMessage)
			return
		}
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "bearer ") {
			writeDemoError(w, http.StatusForbidden, demoCodeTokens, "API tokens are not available in the demo.")
			return
		}
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			writeDemoError(w, http.StatusForbidden, demoCodeDisabled, demoDisabledMessage)
			return
		}

		switch r.Method {
		case http.MethodOptions:
			next.ServeHTTP(w, r)
		case http.MethodGet, http.MethodHead:
			if demoReadDenied(p) || demoHasSegment(p, "exec") {
				writeDemoError(w, http.StatusForbidden, demoCodeDisabled, demoDisabledMessage)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), demoCtxKey{}, true)))
		default:
			if !demoWriteAllowed(r.Method, p) {
				writeDemoError(w, http.StatusForbidden, demoCodeReadonly, demoReadonlyMessage)
				return
			}
			if r.ContentLength > demoBodyLimit {
				writeDemoError(w, http.StatusRequestEntityTooLarge, "demo_body_too_large", "Request body too large.")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, demoBodyLimit)
			next.ServeHTTP(w, r)
		}
	})
}

// demoElevates reports whether a root-only read should be decided as a plain
// read: only the visitor account, only GET or HEAD, and never for any other
// ability (sensitive reads, writes and deploys are never elevated).
func (rt *Router) demoElevates(ctx context.Context, principalType, principalID string) bool {
	if rt.demo == nil {
		return false
	}
	readOnly, _ := ctx.Value(demoCtxKey{}).(bool)
	return readOnly && principalType == store.PrincipalTypeUser && principalID == rt.demo.cfg.VisitorUserID
}

func demoHasSegment(p, seg string) bool {
	for _, s := range strings.Split(p, "/") {
		if s == seg {
			return true
		}
	}
	return false
}
