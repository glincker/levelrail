// TestAIControl_Live_AdminModeBlocksAgentAllowsReal is this package's e2e
// proof for the AI control gate (internal/api/ai_control_gate.go): a real
// agent token over real HTTP is stopped where admin mode must stop it
// and let through where it must not, against a real store.DB. No Docker
// is involved because the gate and the freeze route run before any
// container work.
package e2e

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eAIControlAdminUsername = "e2e-ai-control-admin"
	e2eAIControlAdminPassword = "e2e-ai-control-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eAIControlDevApp        = "levelrail-test-e2e-aictl-dev"
	e2eAIControlProdApp       = "levelrail-test-e2e-aictl-prod"
	e2eAIControlFreezeBody    = `{"windows":[{"cron":"* * * * *","duration":"2h","reason":"agent freeze"}]}`
)

// agentDo sends one request carrying only the agent bearer token, so the
// gate sees an agent class caller and no session cookie.
func agentDo(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader) //nolint:noctx // test helper, loopback-only url
	if err != nil {
		t.Fatalf("NewRequest(%s %s) error = %v", method, url, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: e2eHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing response body: %v", closeErr)
		}
	}()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return resp.StatusCode, string(b)
}

func TestAIControl_Live_AdminModeBlocksAgentAllowsReal(t *testing.T) {
	experimental.Set(experimental.AIControl)
	t.Cleanup(experimental.Reset)

	db := openLiveStore(t)
	ctx := context.Background()

	for name, env := range map[string]string{e2eAIControlDevApp: "env_dev", e2eAIControlProdApp: "env_production"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "nginx:aictl", Port: 8080}); err != nil {
			t.Fatalf("SaveDesiredService(%q) error = %v", name, err)
		}
		if err := db.SetServiceEnvironment(ctx, name, env); err != nil {
			t.Fatalf("SetServiceEnvironment(%q) error = %v", name, err)
		}
	}

	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(discardTestLogger(), b, db, api.WithDeploySafety(db, nil))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, db, e2eAIControlAdminUsername, e2eAIControlAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, ts.URL, e2eAIControlAdminUsername, e2eAIControlAdminPassword)

	// Mint through the real token route so the agent name is stored the
	// way an operator would create it.
	status, body := postJSON(t, admin, ts.URL+"/api/v1/auth/tokens", `{"name":"e2e-ai-control-agent","abilities":["root"],"agent":{"name":"e2e-agent"}}`)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("mint agent token: status = %d, body = %s", status, body)
	}
	token := decodeField(t, []byte(body), "token")

	setMode := func(mode string) {
		t.Helper()
		st, raw := requestJSON(t, admin, http.MethodPut, ts.URL+"/api/v1/settings/ai-control", `{"mode":"`+mode+`","allowed_env_kinds":["dev"]}`)
		if st != http.StatusOK {
			t.Fatalf("set mode %s: status = %d, body = %s", mode, st, raw)
		}
	}
	freezeURL := func(app string) string { return ts.URL + "/api/v1/apps/" + app + "/deploy-freeze" }

	setMode("admin")

	// A real action in an allowed environment goes through.
	if st, raw := agentDo(t, http.MethodPut, freezeURL(e2eAIControlDevApp), token, e2eAIControlFreezeBody); st != http.StatusOK {
		t.Fatalf("admin mode, dev app: status = %d, want 200, body = %s", st, raw)
	}
	// The same action outside the allowed kinds is stopped.
	if st, raw := agentDo(t, http.MethodPut, freezeURL(e2eAIControlProdApp), token, e2eAIControlFreezeBody); st != http.StatusForbidden || !strings.Contains(raw, "not allowed in this environment") {
		t.Fatalf("admin mode, production app: status = %d, want 403 env limit, body = %s", st, raw)
	}
	// Admin mode still never lets an agent change its own guardrail or approve a deploy.
	if st, raw := agentDo(t, http.MethodPut, ts.URL+"/api/v1/settings/ai-control", token, `{"mode":"admin","allowed_env_kinds":["dev","production"]}`); st != http.StatusForbidden || !strings.Contains(raw, "cannot change AI control") {
		t.Fatalf("agent self-admin: status = %d, want 403, body = %s", st, raw)
	}
	if st, raw := agentDo(t, http.MethodPost, ts.URL+"/api/v1/deploy-approvals/da_missing/approve", token, ""); st != http.StatusForbidden || !strings.Contains(raw, "a human must") {
		t.Fatalf("agent approve: status = %d, want 403, body = %s", st, raw)
	}

	// A person tightens the mode and the same write is now refused.
	setMode("observe")
	if st, raw := agentDo(t, http.MethodPut, freezeURL(e2eAIControlDevApp), token, e2eAIControlFreezeBody); st != http.StatusForbidden || !strings.Contains(raw, "limited by the current AI control mode") {
		t.Fatalf("observe mode, write: status = %d, want 403 mode limit, body = %s", st, raw)
	}
}
