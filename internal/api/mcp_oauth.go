package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	mcpScopeRead          = "mcp:read"
	mcpScopeReadSensitive = "mcp:read:sensitive"

	envMCPOAuthCodeTTL    = "APP_MCP_OAUTH_CODE_TTL"
	envMCPOAuthRequestTTL = "APP_MCP_OAUTH_REQUEST_TTL"
	envMCPOAuthAccessTTL  = "APP_MCP_OAUTH_ACCESS_TTL"
	envMCPOAuthRefreshTTL = "APP_MCP_OAUTH_REFRESH_TTL"
	envMCPOAuthGrantMax   = "APP_MCP_OAUTH_GRANT_MAX_AGE"
	envMCPOAuthTokenRPM   = "APP_MCP_OAUTH_TOKEN_RPM" //nolint:gosec // env var name

	mcpOAuthDefaultCodeTTL    = time.Minute
	mcpOAuthDefaultRequestTTL = 10 * time.Minute
	mcpOAuthDefaultAccessTTL  = time.Hour
	mcpOAuthDefaultRefreshTTL = 30 * 24 * time.Hour
	mcpOAuthDefaultGrantMax   = 90 * 24 * time.Hour
	mcpOAuthDefaultTokenRPM   = 30

	mcpOAuthAgentPrefix = "mcp:"
	mcpOAuthConsentPath = "/oauth/consent"
	mcpOAuthMaxAgentLen = 64

	// AuditActionMCPOAuthGrant is a grant approved or its first token issued.
	AuditActionMCPOAuthGrant = "mcp.oauth.grant_created"
	// AuditActionMCPOAuthDenied is a consent request the user refused.
	AuditActionMCPOAuthDenied = "mcp.oauth.denied"
	// AuditActionMCPOAuthReuse is a replayed code or refresh token that ended a grant.
	AuditActionMCPOAuthReuse = "mcp.oauth.refresh_reuse"
)

// mcpOAuthStore is the persistence the authorization server needs. It is
// satisfied by the real database; routes answer 501 for a router without it.
type mcpOAuthStore interface {
	CreateMCPOAuthClient(ctx context.Context, c store.MCPOAuthClient) error
	GetMCPOAuthClient(ctx context.Context, id string) (*store.MCPOAuthClient, error)
	ListMCPOAuthClients(ctx context.Context) ([]store.MCPOAuthClient, error)
	DeleteMCPOAuthClient(ctx context.Context, id string, now int64) ([]string, error)
	CreateMCPOAuthRequest(ctx context.Context, r store.MCPOAuthRequest, now int64) error
	GetMCPOAuthRequest(ctx context.Context, id string) (*store.MCPOAuthRequest, error)
	DecideMCPOAuthRequest(ctx context.Context, id, userID string, approved bool, scopes []string, codeHash string, codeExpires, now int64) (bool, error)
	ConsumeMCPOAuthCode(ctx context.Context, codeHash string, now int64) (*store.MCPOAuthRequest, bool, error)
	CreateMCPOAuthGrant(ctx context.Context, g store.MCPOAuthGrant, tokenID, refreshHash string, refreshExpires, now int64) error
	GetMCPOAuthGrant(ctx context.Context, id string) (*store.MCPOAuthGrant, error)
	GetMCPOAuthGrantByToken(ctx context.Context, tokenID string) (*store.MCPOAuthGrant, error)
	GetMCPOAuthGrantByRefresh(ctx context.Context, hash string) (*store.MCPOAuthGrant, error)
	GetMCPOAuthGrantByRequest(ctx context.Context, requestID string) (*store.MCPOAuthGrant, error)
	RotateMCPOAuthRefresh(ctx context.Context, oldHash, newHash string, newExpires int64, newTokenID string, now int64) (store.MCPRefreshOutcome, string, error)
	RevokeMCPOAuthGrant(ctx context.Context, grantID string, now int64) ([]string, error)
	MCPOAuthGrantTokenIDs(ctx context.Context, grantID string) ([]string, error)
}

func (rt *Router) mcpOAuth() mcpOAuthStore {
	s, _ := any(rt.apps).(mcpOAuthStore)
	return s
}

type mcpAudienceKey struct{}

// withMCPAudience marks a context as belonging to the MCP resource. Only code
// inside this package can set it, so a client cannot forge the audience.
func withMCPAudience(ctx context.Context, resource string) context.Context {
	return context.WithValue(ctx, mcpAudienceKey{}, resource)
}

func mcpAudienceFrom(ctx context.Context) string {
	v, _ := ctx.Value(mcpAudienceKey{}).(string)
	return v
}

// mcpOAuthAudienceOK is false for an OAuth-issued token presented anywhere
// except the MCP resource it was bound to, or whose grant was revoked.
func (rt *Router) mcpOAuthAudienceOK(ctx context.Context, rec *store.APIToken) bool {
	if !strings.HasPrefix(rec.AgentName, mcpOAuthAgentPrefix) {
		return true
	}
	st := rt.mcpOAuth()
	if st == nil {
		return true
	}
	g, err := st.GetMCPOAuthGrantByToken(ctx, rec.ID)
	if errors.Is(err, store.ErrMCPOAuthNotFound) {
		return true
	}
	if err != nil || g.Revoked {
		return false
	}
	aud := mcpAudienceFrom(ctx)
	return aud != "" && subtle.ConstantTimeCompare([]byte(aud), []byte(g.Resource)) == 1
}

// mcpIssuer is the public base URL. It comes from configuration, never from the
// Host header, except for loopback development.
func mcpIssuer(r *http.Request) (string, bool) {
	if d := configuredDashboardURL(); d != "" {
		return d, true
	}
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); host == "localhost" || (ip != nil && ip.IsLoopback()) {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + r.Host, true
	}
	return "", false
}

func mcpResourceURL(issuer string) string { return issuer + mcpHTTPPath }

func mcpMetadataURL(issuer string) string {
	return issuer + "/.well-known/oauth-protected-resource"
}

// mcpChallenge is the WWW-Authenticate value that points a client at discovery.
func mcpChallenge(r *http.Request) string {
	if issuer, ok := mcpIssuer(r); ok && mcpOAuthOn() {
		return `Bearer realm="levelrail-mcp", resource_metadata="` + mcpMetadataURL(issuer) + `"`
	}
	return `Bearer realm="levelrail-mcp"`
}

func mcpOAuthOn() bool { return featureOn(experimental.MCPHTTP) }

func mcpOAuthScopes() []string { return []string{mcpScopeRead, mcpScopeReadSensitive} }

func mcpScopeAbility(scope string) string {
	switch scope {
	case mcpScopeRead:
		return AbilityRead
	case mcpScopeReadSensitive:
		return AbilityReadSensitive
	}
	return ""
}

// mcpGrantable caps requested scopes at what the user holds. write:sensitive,
// root and every write tier stay ungrantable in this slice.
func mcpGrantable(requested, userAbilities []string) []string {
	var out []string
	for _, s := range requested {
		if a := mcpScopeAbility(s); a != "" && hasAbility(userAbilities, a) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func mcpAbilitiesFor(scopes []string) []string {
	out := []string{AbilityRead}
	if slices.Contains(scopes, mcpScopeReadSensitive) {
		out = append(out, AbilityReadSensitive)
	}
	return out
}

func parseMCPScopes(raw string) ([]string, bool) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return []string{mcpScopeRead}, true
	}
	for _, f := range fields {
		if mcpScopeAbility(f) == "" {
			return nil, false
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(fields))), true
}

func randomURLToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// pkceS256 reports whether verifier hashes to challenge, in constant time.
func pkceS256(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	for _, c := range verifier {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)
		if !ok {
			return false
		}
	}
	return subtle.ConstantTimeCompare([]byte(sha256Hex(verifier)), []byte(challenge)) == 1
}

func validPKCEChallenge(c string) bool {
	if len(c) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(c)
	return err == nil
}

func isLoopbackRedirect(u *url.URL) bool {
	if u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	return h == "127.0.0.1" || h == "::1"
}

// validRegisteredRedirect applies the registration rules: absolute, no
// fragment, https except loopback http for native clients.
func validRegisteredRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Fragment != "" || u.Host == "" || u.User != nil {
		return false
	}
	return u.Scheme == "https" || isLoopbackRedirect(u)
}

// redirectMatches is an exact match, except a registered loopback URI also
// accepts any port (RFC 8252).
func redirectMatches(registered []string, got string) bool {
	if slices.Contains(registered, got) {
		return true
	}
	gu, err := url.Parse(got)
	if err != nil || !isLoopbackRedirect(gu) || gu.Fragment != "" {
		return false
	}
	for _, reg := range registered {
		ru, err := url.Parse(reg)
		if err != nil || !isLoopbackRedirect(ru) {
			continue
		}
		if ru.Hostname() == gu.Hostname() && ru.Path == gu.Path && ru.RawQuery == gu.RawQuery {
			return true
		}
	}
	return false
}

// mcpDiscovery is the shared guard of the unauthenticated discovery and OAuth routes.
func (rt *Router) mcpOAuthReady(w http.ResponseWriter, r *http.Request) (mcpOAuthStore, string, bool) {
	st := rt.mcpOAuth()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "OAuth for remote MCP is not available on this control plane")
		return nil, "", false
	}
	issuer, ok := mcpIssuer(r)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "set APP_DASHBOARD_URL to the public HTTPS URL before using OAuth for remote MCP")
		return nil, "", false
	}
	return st, issuer, true
}

func (rt *Router) handleMCPProtectedResource(w http.ResponseWriter, r *http.Request) {
	_, issuer, ok := rt.mcpOAuthReady(w, r)
	if !ok {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 mcpResourceURL(issuer),
		"authorization_servers":    []string{issuer},
		"scopes_supported":         mcpOAuthScopes(),
		"bearer_methods_supported": []string{"header"},
	})
}

func (rt *Router) handleMCPAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	_, issuer, ok := rt.mcpOAuthReady(w, r)
	if !ok {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         issuer,
		"authorization_endpoint":                         issuer + "/oauth/authorize",
		"token_endpoint":                                 issuer + "/oauth/token",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"scopes_supported":                               mcpOAuthScopes(),
		"authorization_response_iss_parameter_supported": true,
	})
}

// mcpOAuthAudit writes one audit row for a grant lifecycle event.
func (rt *Router) mcpOAuthAudit(ctx context.Context, r *http.Request, action, actorType, actorID string, detail map[string]any) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	d, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: rt.auditActorName(ctx, actorType, actorID, ""),
		Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action, Detail: string(d),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: save mcp oauth audit entry failed", "action", action)
	}
}

func nowUnix() int64 { return time.Now().Unix() }

// mcpOAuthAgentName is the agent name every OAuth-issued token carries, so AI
// control always governs a remote caller.
func mcpOAuthAgentName(clientName string) string {
	return mcpTruncate(mcpOAuthAgentPrefix+clientName, mcpOAuthMaxAgentLen)
}

func envMCPOAuthTokenLimit() int { return envPositiveInt(envMCPOAuthTokenRPM, mcpOAuthDefaultTokenRPM) }
