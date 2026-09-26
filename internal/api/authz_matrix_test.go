package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type matrixRoute struct {
	method  string
	pattern string
}

func (m matrixRoute) key() string { return m.method + " " + m.pattern }

var (
	matrixRouteRe = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) ([^"]+)"`)
	matrixParamRe = regexp.MustCompile(`\{[^}]+\}`)
)

// publicRoutes are the only routes allowed to answer an anonymous caller
// with something other than 401. Adding a route here needs a reason a
// reviewer can challenge.
var publicRoutes = map[string]string{ //nolint:gosec // route paths, not credentials
	"GET /healthz":                               "liveness probe for systemd and load balancers",
	"GET /readyz":                                "readiness probe, no data beyond ready/not ready",
	"GET /api/v1/brand":                          "login screen needs branding before a session exists",
	"GET /api/v1/dev-mode":                       "login screen banner, boolean only",
	"POST /api/v1/auth/login":                    "credential exchange, rate limited",
	"POST /api/v1/auth/register":                 "first-run setup, refuses once an admin exists",
	"GET /api/v1/auth/setup-status":              "tells the SPA whether to show first-run setup",
	"POST /api/v1/auth/2fa/verify":               "step two of login, gated by an MFA pending token and rate limit",
	"POST /api/v1/invites/accept":                "authenticated by the single-use invite token in the body",
	"POST /api/v1/auth/device/start":             "CLI device login start, rate limited",
	"POST /api/v1/auth/device/token":             "CLI device login poll, authenticated by the device code",
	"GET /api/v1/auth/oauth/providers":           "login screen lists enabled providers",
	"GET /api/v1/auth/oauth/{provider}/start":    "OAuth sign-in redirect",
	"GET /api/v1/auth/oauth/{provider}/callback": "OAuth sign-in callback, authenticated by state and code",
	"POST /api/v1/auth/forgot-password":          "always generic response, rate limited",
	"POST /api/v1/auth/reset-password":           "authenticated by the single-use reset token",
	"POST /api/v1/webhooks/github/{name}":        "authenticated by the HMAC signature of the app's webhook secret",
	"GET /public/status":                         "opt-in public status page, serves only operator-chosen names and statuses, rate limited and cacheable",
	"GET /public/status.json":                    "JSON form of the opt-in public status page, same whitelisted view",
	"GET /public/status.rss":                     "RSS feed of operator-authored incidents on the opt-in public status page",
}

// readOnlyMayMutate lists mutating routes a read-only token may call:
// public routes (not token-gated at all) and pure computations that
// change no state.
var readOnlyMayMutate = map[string]string{
	"POST /api/v1/pipelines/validate":    "validates YAML, persists nothing",
	"POST /api/v1/apps/{name}/preflight": "read-only probes of the stored app config, persists nothing",
	"POST /api/v1/apply/plan":            "computes a plan from live reads, persists nothing",
	"POST /api/v1/prometheus/read":       "Prometheus remote read is a POST but only queries, gated by AbilityRead",
}

// denyExempt lists /apps/{name}/... routes that legitimately do not
// honour a per-app IAM Deny, with the reason.
var denyExempt = map[string]string{}

func loadMatrixRoutes(t *testing.T) []matrixRoute {
	t.Helper()
	files, err := filepath.Glob("routes*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no routes files found: %v", err)
	}
	var out []matrixRoute
	for _, f := range files {
		src, err := os.ReadFile(f) //nolint:gosec // globbed routes files in the package dir
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range matrixRouteRe.FindAllStringSubmatch(string(src), -1) {
			out = append(out, matrixRoute{method: m[1], pattern: m[2]})
		}
	}
	if len(out) < 100 {
		t.Fatalf("parsed only %d routes, the route regex has drifted", len(out))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

// concretePath fills a route pattern's path parameters. The resource
// name segment for /apps, /databases and /models becomes name so a
// resource-scoped Deny can target it.
func concretePath(pattern, name string) string {
	first := true
	return matrixParamRe.ReplaceAllStringFunc(pattern, func(p string) string {
		if p == "{name}" && first {
			first = false
			return name
		}
		return "x1"
	})
}

func matrixDo(rt *Router, method, path string, mutate func(*http.Request)) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, method, path, nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func bearer(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func seedMatrixToken(t *testing.T, db *store.DB, id string, abilities []string) string {
	t.Helper()
	plain := "matrix-secret-" + id
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return plain
}

func TestAuthzMatrix_PublicAllowlistIsCurrent(t *testing.T) {
	registered := map[string]bool{}
	for _, r := range loadMatrixRoutes(t) {
		registered[r.key()] = true
	}
	for k := range publicRoutes {
		if !registered[k] {
			t.Errorf("public allowlist entry %q is not a registered route, remove it", k)
		}
	}
	for k := range readOnlyMayMutate {
		if !registered[k] {
			t.Errorf("readOnlyMayMutate entry %q is not a registered route, remove it", k)
		}
	}
}

func TestAuthzMatrix_UnauthenticatedGets401(t *testing.T) {
	rt, _, _ := newPipelineRouter(t)
	var bad []string
	for _, r := range loadMatrixRoutes(t) {
		if _, public := publicRoutes[r.key()]; public {
			continue
		}
		if got := matrixDo(rt, r.method, concretePath(r.pattern, "web"), nil); got != http.StatusUnauthorized {
			bad = append(bad, r.key()+" -> "+http.StatusText(got))
		}
	}
	if len(bad) > 0 {
		t.Errorf("routes that do not answer an anonymous request with 401 (add auth or a reasoned publicRoutes entry):\n%s", strings.Join(bad, "\n"))
	}
}

func TestAuthzMatrix_ReadOnlyTokenCannotMutate(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	tok := seedMatrixToken(t, db, "ro", []string{AbilityRead})
	var bad []string
	for _, r := range loadMatrixRoutes(t) {
		switch r.method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			continue
		}
		if _, ok := publicRoutes[r.key()]; ok {
			continue
		}
		if _, ok := readOnlyMayMutate[r.key()]; ok {
			continue
		}
		// Session-only routes (requireAuth) answer a bearer token 401, not 403.
		got := matrixDo(rt, r.method, concretePath(r.pattern, "web"), bearer(tok))
		if got != http.StatusForbidden && got != http.StatusUnauthorized {
			bad = append(bad, r.key()+" -> "+http.StatusText(got))
		}
	}
	if len(bad) > 0 {
		t.Errorf("mutating routes reachable with a read-only token:\n%s", strings.Join(bad, "\n"))
	}
}

// resourceKinds maps a path prefix to the IAM resource prefix its routes
// are scoped by.
var resourceKinds = []struct {
	pathPrefix string
	iamPrefix  string
	label      string
}{
	{"/api/v1/apps/{name}", "app:", "app"},
	{"/api/v1/databases/{name}", "database:", "database"},
	{"/api/v1/models/{name}", "model:", "model"},
}

func TestAuthzMatrix_ResourceDenyReturns403(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	user := storeUserWithAbilitiesForTest(t, db, "deny-matrix@example.com", []string{AbilityRoot})
	cookie := sessionCookieForTest(t, rt, user.ID)
	tokPlain := seedMatrixToken(t, db, "deny-tok", []string{AbilityRoot})

	for _, k := range resourceKinds {
		attachTestPolicy(t, db, "deny-u-"+k.label, "Deny", "*", k.iamPrefix+"victim", store.PrincipalTypeUser, user.ID)
		attachTestPolicy(t, db, "deny-t-"+k.label, "Deny", "*", k.iamPrefix+"victim", store.PrincipalTypeToken, "deny-tok")
	}

	for _, k := range resourceKinds {
		t.Run(k.label, func(t *testing.T) {
			var bad []string
			checked := 0
			for _, r := range loadMatrixRoutes(t) {
				if !strings.Contains(r.pattern, k.pathPrefix) {
					continue
				}
				if _, ok := denyExempt[r.key()]; ok {
					continue
				}
				checked++
				victim := concretePath(r.pattern, "victim")
				for name, mutate := range map[string]func(*http.Request){
					"session": func(req *http.Request) { req.AddCookie(cookie) },
					"token":   bearer(tokPlain),
				} {
					got := matrixDo(rt, r.method, victim, mutate)
					if got == http.StatusForbidden {
						continue
					}
					bad = append(bad, r.key()+" ("+name+") -> "+http.StatusText(got))
				}
			}
			if checked == 0 {
				t.Fatalf("no %s routes checked", k.label)
			}
			if len(bad) > 0 {
				t.Errorf("%s routes that ignore a per-resource IAM Deny:\n%s", k.label, strings.Join(bad, "\n"))
			}
		})
	}
}
