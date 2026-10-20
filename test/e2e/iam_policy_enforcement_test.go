// Live proof the IAM resource gate holds when every step, including
// minting the second principal and attaching its policy, goes through
// the real HTTP API, not store.AttachPolicy called directly (that's
// authz_matrix_test.go and require_ability_for_resource_test.go's own
// coverage). POST /apps/{name}/stop only flips
// store.DesiredService.Suspended, so no container needs to exist.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eIAMAdminUsername = "e2e-iam-policy-admin"
	e2eIAMAdminPassword = "e2e-iam-policy-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

// seedIAMTestApp saves a minimal real app directly through the store,
// the same shortcut TestRequireAbilityForResource_SessionDenyPolicyOverridesBaseAbility
// (internal/api/require_ability_for_resource_test.go) takes: what's
// under test here is the IAM gate in front of the app, not app
// creation itself, which already has its own deploy-path coverage.
func seedIAMTestApp(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: name, Image: "levelrail/iam-e2e:1", Port: 8080,
	}); err != nil {
		t.Fatalf("seed app %q: %v", name, err)
	}
}

// createRealUser mints a second principal through the real
// POST /api/v1/auth/users route (not CreateUser called on the store
// directly) and returns its ID, the same way an admin would hand a
// teammate an account before scoping it with a policy.
func createRealUser(t *testing.T, client *http.Client, baseURL, email, password string, abilities []string) string {
	t.Helper()
	abilitiesJSON := `["` + strings.Join(abilities, `","`) + `"]`
	status, body := requestJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/users",
		`{"email":"`+email+`","password":"`+password+`","abilities":`+abilitiesJSON+`}`)
	if status != http.StatusCreated {
		t.Fatalf("create user %q: status = %d, want %d, body = %s", email, status, http.StatusCreated, body)
	}
	return decodeField(t, body, "id")
}

// createRealPolicy mints a single-statement policy through the real
// POST /api/v1/iam/policies route and returns its ID.
func createRealPolicy(t *testing.T, client *http.Client, baseURL, name, effect, action, resource string) string {
	t.Helper()
	doc := `{"Statement":[{"Effect":"` + effect + `","Action":["` + action + `"],"Resource":["` + resource + `"]}]}`
	status, body := requestJSON(t, client, http.MethodPost, baseURL+"/api/v1/iam/policies",
		`{"name":"`+name+`","document":`+doc+`}`)
	if status != http.StatusCreated {
		t.Fatalf("create policy %q: status = %d, want %d, body = %s", name, status, http.StatusCreated, body)
	}
	return decodeField(t, body, "id")
}

// attachRealPolicy attaches policyID to a principal through the real
// POST /api/v1/iam/policies/{id}/attachments route, the route an admin
// would actually click through, not store.AttachPolicy called directly.
func attachRealPolicy(t *testing.T, client *http.Client, baseURL, policyID, principalType, principalID string) {
	t.Helper()
	status, body := requestJSON(t, client, http.MethodPost, baseURL+"/api/v1/iam/policies/"+policyID+"/attachments",
		`{"principal_type":"`+principalType+`","principal_id":"`+principalID+`"}`)
	if status != http.StatusNoContent {
		t.Fatalf("attach policy %q to %s %q: status = %d, want %d, body = %s", policyID, principalType, principalID, status, http.StatusNoContent, body)
	}
}

// createRealToken mints an API token through the real
// POST /api/v1/auth/tokens route, session-only by design (router.go),
// and returns its ID (the IAM principal_id a policy attaches to) and
// its one-time plaintext secret.
func createRealToken(t *testing.T, client *http.Client, baseURL, name string, abilities []string) (id, plaintext string) {
	t.Helper()
	abilitiesJSON := `["` + strings.Join(abilities, `","`) + `"]`
	status, body := requestJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/tokens",
		`{"name":"`+name+`","abilities":`+abilitiesJSON+`}`)
	if status != http.StatusCreated {
		t.Fatalf("create token %q: status = %d, want %d, body = %s", name, status, http.StatusCreated, body)
	}
	return decodeField(t, body, "id"), decodeField(t, body, "token")
}

// decodeField pulls one top-level string field out of a JSON response
// body, the minimal decode every helper above needs (an id, or a
// token's one-time plaintext) without each defining its own throwaway
// struct for a single field.
func decodeField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode response %s: %v", body, err)
	}
	v, ok := decoded[field].(string)
	if !ok || v == "" {
		t.Fatalf("response %s has no non-empty %q field", body, field)
	}
	return v
}

// bearerStop issues POST /api/v1/apps/{name}/stop with token as a
// bearer credential: requestJSON (auth_lifecycle_test.go) only ever
// carries a session cookie jar, so a token-authenticated call needs its
// own request builder with an Authorization header instead.
func bearerStop(t *testing.T, baseURL, appName, token string) (status int, body []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/apps/"+appName+"/stop", nil) //nolint:noctx // test helper, url is loopback-only
	if err != nil {
		t.Fatalf("NewRequest(POST stop %s) error = %v", appName, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: e2eHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatalf("POST stop %s: %v", appName, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing response body: %v", closeErr)
		}
	}()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return resp.StatusCode, body
}

// TestIAMPolicyEnforcement_Live_SessionDenyBlocksRealAction proves a
// Deny policy, attached to a real second user through the real IAM API,
// genuinely blocks that user's session from stopping one specific app
// over real HTTP, while the identical action against a different app
// the policy never mentions still succeeds for the same session.
func TestIAMPolicyEnforcement_Live_SessionDenyBlocksRealAction(t *testing.T) {
	db := openLiveStore(t)
	logger := discardTestLogger()
	b := &brand.Brand{Name: "IAM E2E Test Platform", BinaryName: "iam-e2e-test-platform"}
	router := api.NewRouter(logger, b, db)
	server := newE2ETestServer(t, router)

	const victim, safe = "iam-e2e-victim", "iam-e2e-safe"
	seedIAMTestApp(t, db, victim)
	seedIAMTestApp(t, db, safe)

	if err := api.BootstrapAdmin(context.Background(), db, e2eIAMAdminUsername, e2eIAMAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, server.URL, e2eIAMAdminUsername, e2eIAMAdminPassword)

	const (
		deployerEmail    = "e2e-iam-deployer@example.com"
		deployerPassword = "e2e-iam-deployer-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	)
	userID := createRealUser(t, admin, server.URL, deployerEmail, deployerPassword, []string{"deploy"})

	policyID := createRealPolicy(t, admin, server.URL, "deny-victim-stop", "Deny", "deploy", "app:"+victim)
	attachRealPolicy(t, admin, server.URL, policyID, store.PrincipalTypeUser, userID)

	deployer := loginE2EClient(t, server.URL, deployerEmail, deployerPassword)

	status, body := requestJSON(t, deployer, http.MethodPost, server.URL+"/api/v1/apps/"+victim+"/stop", "")
	if status != http.StatusForbidden {
		t.Fatalf("stop %q (denied app): status = %d, want %d, body = %s", victim, status, http.StatusForbidden, body)
	}

	status, body = requestJSON(t, deployer, http.MethodPost, server.URL+"/api/v1/apps/"+safe+"/stop", "")
	if status != http.StatusOK {
		t.Fatalf("stop %q (unaffected app, same session): status = %d, want %d, body = %s", safe, status, http.StatusOK, body)
	}

	svc, err := db.GetDesiredService(context.Background(), safe)
	if err != nil {
		t.Fatalf("GetDesiredService(%q) error = %v", safe, err)
	}
	if !svc.Suspended {
		t.Errorf("GetDesiredService(%q).Suspended = false, want true: the real 200 above must correspond to a real state change", safe)
	}

	svc, err = db.GetDesiredService(context.Background(), victim)
	if err != nil {
		t.Fatalf("GetDesiredService(%q) error = %v", victim, err)
	}
	if svc.Suspended {
		t.Errorf("GetDesiredService(%q).Suspended = true, want false: the real 403 above must correspond to no state change at all", victim)
	}
}

// TestIAMPolicyEnforcement_Live_TokenAllowScopesRealAction proves the
// other half of the same story: a bearer token minted with only
// AbilityRead, no deploy ability at all, gains the ability to stop one
// specific app purely from a resource-scoped Allow policy attached to
// it through the real IAM API, while the same token still cannot touch
// any app the policy does not name.
func TestIAMPolicyEnforcement_Live_TokenAllowScopesRealAction(t *testing.T) {
	db := openLiveStore(t)
	logger := discardTestLogger()
	b := &brand.Brand{Name: "IAM E2E Test Platform", BinaryName: "iam-e2e-test-platform"}
	router := api.NewRouter(logger, b, db)
	server := newE2ETestServer(t, router)

	const scoped, uncovered = "iam-e2e-scoped", "iam-e2e-uncovered"
	seedIAMTestApp(t, db, scoped)
	seedIAMTestApp(t, db, uncovered)

	if err := api.BootstrapAdmin(context.Background(), db, e2eIAMAdminUsername, e2eIAMAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, server.URL, e2eIAMAdminUsername, e2eIAMAdminPassword)

	tokenID, tokenPlain := createRealToken(t, admin, server.URL, "iam-e2e-scoped-token", []string{"read"})

	policyID := createRealPolicy(t, admin, server.URL, "allow-scoped-stop", "Allow", "deploy", "app:"+scoped)
	attachRealPolicy(t, admin, server.URL, policyID, store.PrincipalTypeToken, tokenID)

	status, body := bearerStop(t, server.URL, scoped, tokenPlain)
	if status != http.StatusOK {
		t.Fatalf("stop %q (allow-scoped token): status = %d, want %d, body = %s", scoped, status, http.StatusOK, body)
	}

	status, body = bearerStop(t, server.URL, uncovered, tokenPlain)
	if status != http.StatusForbidden {
		t.Fatalf("stop %q (same token, app the policy never names): status = %d, want %d, body = %s", uncovered, status, http.StatusForbidden, body)
	}

	svc, err := db.GetDesiredService(context.Background(), scoped)
	if err != nil {
		t.Fatalf("GetDesiredService(%q) error = %v", scoped, err)
	}
	if !svc.Suspended {
		t.Errorf("GetDesiredService(%q).Suspended = false, want true: the real 200 above must correspond to a real state change", scoped)
	}

	svc, err = db.GetDesiredService(context.Background(), uncovered)
	if err != nil {
		t.Fatalf("GetDesiredService(%q) error = %v", uncovered, err)
	}
	if svc.Suspended {
		t.Errorf("GetDesiredService(%q).Suspended = true, want false: the real 403 above must correspond to no state change at all", uncovered)
	}
}
