// TestFeatureFlags_Live_APICLIAndMCP drives feature flags through every
// operator surface against one real HTTP server and a real store: the
// REST API (create, evaluate with a read-only token, kill switch,
// rollout, per-app scoping, delete), the real levelrail-cli binary, and
// the real MCP tools over an in-memory transport. Per-environment
// targeting does not exist in the product (docs/feature-flags.md lists
// what is unsupported), so scoping is asserted per app, the only scope
// flags have.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eFlagsAdminUsername = "e2e-flags-admin"
	e2eFlagsAdminPassword = "e2e-flags-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eFlagsAppA          = "levelrail-test-e2e-flags-a"
	e2eFlagsAppB          = "levelrail-test-e2e-flags-b"
)

type e2eFlag struct {
	ID                string `json:"id"`
	Key               string `json:"key"`
	Name              string `json:"name"`
	ServiceName       string `json:"service_name"`
	Enabled           bool   `json:"enabled"`
	RolloutPercentage int    `json:"rollout_percentage"`
}

func TestFeatureFlags_Live_APICLIAndMCP(t *testing.T) {
	db := openLiveStore(t)
	ctx := context.Background()

	for _, app := range []string{e2eFlagsAppA, e2eFlagsAppB} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: "nginx:flags", Port: 8080}); err != nil {
			t.Fatalf("SaveDesiredService(%s) error = %v", app, err)
		}
	}
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	ts := newE2ETestServer(t, api.NewRouter(discardTestLogger(), b, db))
	if err := api.BootstrapAdmin(ctx, db, e2eFlagsAdminUsername, e2eFlagsAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, ts.URL, e2eFlagsAdminUsername, e2eFlagsAdminPassword)

	writeToken := createE2EToken(t, admin, ts.URL, "e2e-flags-write", `["read","write"]`)
	readToken := createE2EToken(t, admin, ts.URL, "e2e-flags-read", `["read"]`)

	evaluate := func(token, key, identifier string) (int, bool) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/flags/evaluate/"+key+"?identifier="+identifier, nil) //nolint:noctx // test helper
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := (&http.Client{Timeout: e2eHTTPTimeout}).Do(req)
		if err != nil {
			t.Fatalf("evaluate %s: %v", key, err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out struct {
			Key     string `json:"key"`
			Enabled bool   `json:"enabled"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out.Enabled
	}
	expectEval := func(key, identifier string, want bool) {
		t.Helper()
		status, got := evaluate(readToken, key, identifier)
		if status != http.StatusOK || got != want {
			t.Fatalf("evaluate %s id=%s: status=%d enabled=%t, want 200 enabled=%t", key, identifier, status, got, want)
		}
	}
	flagsURL := func(app string) string { return ts.URL + "/api/v1/apps/" + app + "/flags" }

	// API: create, evaluate with a read-only token, mutate, delete.
	status, body := postJSON(t, admin, flagsURL(e2eFlagsAppA), `{"key":"e2e-new-checkout","name":"New checkout","enabled":true,"rollout_percentage":100}`)
	if status != http.StatusCreated {
		t.Fatalf("create flag: status = %d, body = %s", status, body)
	}
	var created e2eFlag
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.ID == "" || created.ServiceName != e2eFlagsAppA {
		t.Fatalf("create flag: decode %s: %v", body, err)
	}
	expectEval("e2e-new-checkout", "user-1", true)

	if status, _ := postJSON(t, admin, flagsURL(e2eFlagsAppB), `{"key":"e2e-new-checkout","name":"Dup","enabled":true,"rollout_percentage":100}`); status != http.StatusConflict {
		t.Fatalf("duplicate key on another app: status = %d, want %d", status, http.StatusConflict)
	}
	if status, _ := evaluate(readToken, "e2e-no-such-flag", "u"); status != http.StatusNotFound {
		t.Fatalf("evaluate unknown key: status = %d, want 404", status)
	}

	flagURL := flagsURL(e2eFlagsAppA) + "/" + created.ID
	put := func(payload string) {
		t.Helper()
		if status, raw := requestJSON(t, admin, http.MethodPut, flagURL, payload); status != http.StatusOK {
			t.Fatalf("update flag: status = %d, body = %s", status, raw)
		}
	}
	put(`{"name":"New checkout","enabled":false,"rollout_percentage":100}`)
	expectEval("e2e-new-checkout", "user-1", false)
	put(`{"name":"New checkout","enabled":true,"rollout_percentage":0}`)
	expectEval("e2e-new-checkout", "user-1", false)

	put(`{"name":"New checkout","enabled":true,"rollout_percentage":50}`)
	on, off := 0, 0
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("user-%d", i)
		_, first := evaluate(readToken, "e2e-new-checkout", id)
		_, second := evaluate(readToken, "e2e-new-checkout", id)
		if first != second {
			t.Fatalf("identifier %s flipped between identical evaluations", id)
		}
		if first {
			on++
		} else {
			off++
		}
	}
	if on < 50 || off < 50 {
		t.Fatalf("50%% rollout over 200 identifiers gave on=%d off=%d, want a roughly even split", on, off)
	}

	put(`{"name":"New checkout","enabled":true,"rollout_percentage":100}`)

	// A read-only token must not be able to change a flag.
	req, err := http.NewRequest(http.MethodPut, flagURL, strings.NewReader(`{"name":"x","enabled":false,"rollout_percentage":0}`)) //nolint:noctx // test helper
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+readToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: e2eHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatalf("read-only PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only token PUT: status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	expectEval("e2e-new-checkout", "user-1", true)

	// CLI: the real binary against the real server, on the second app.
	cli := buildE2ECLI(t)
	runCLI := func(args ...string) string {
		t.Helper()
		full := append(append([]string{}, args[:1]...), "--api-url", ts.URL, "--token", writeToken, "--json")
		full = append(full, args[1:]...)
		cmd := exec.Command(cli, append([]string{"flags"}, full...)...) //nolint:gosec,noctx // test-built binary, fixed args
		cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cli flags %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	var cliFlag e2eFlag
	if err := json.Unmarshal([]byte(runCLI("create", "--key", "e2e-cli-flag", "--name", "CLI flag", "--rollout", "100", e2eFlagsAppB)), &cliFlag); err != nil || cliFlag.ID == "" {
		t.Fatalf("cli create: decode: %v", err)
	}
	expectEval("e2e-cli-flag", "user-1", true)
	if out := runCLI("list", e2eFlagsAppB); !strings.Contains(out, "e2e-cli-flag") || strings.Contains(out, "e2e-new-checkout") {
		t.Fatalf("cli list for app B = %s, want only its own flag", out)
	}
	runCLI("set", "--name", "CLI flag", "--disabled", e2eFlagsAppB, cliFlag.ID)
	expectEval("e2e-cli-flag", "user-1", false)
	if out := runCLI("get", e2eFlagsAppB, cliFlag.ID); !strings.Contains(out, `"enabled": false`) && !strings.Contains(out, `"enabled":false`) {
		t.Fatalf("cli get after disable = %s, want enabled false", out)
	}

	// MCP: the read tools over the same server.
	session := newE2EMCPSession(t, ts.URL, readToken)
	listed := callE2EMCP(t, session, "list_feature_flags", map[string]any{"name": e2eFlagsAppA})
	var mcpFlags []e2eFlag
	if err := json.Unmarshal(listed, &mcpFlags); err != nil || len(mcpFlags) != 1 || mcpFlags[0].Key != "e2e-new-checkout" {
		t.Fatalf("mcp list_feature_flags = %s (err %v), want only app A's flag", listed, err)
	}
	var mcpOne e2eFlag
	if err := json.Unmarshal(callE2EMCP(t, session, "get_feature_flag", map[string]any{"name": e2eFlagsAppA, "id": created.ID}), &mcpOne); err != nil || mcpOne.ID != created.ID || !mcpOne.Enabled {
		t.Fatalf("mcp get_feature_flag = %+v (err %v)", mcpOne, err)
	}

	// Delete through the CLI and the API, then evaluate and list.
	runCLI("delete", e2eFlagsAppB, cliFlag.ID)
	if status, _ := evaluate(readToken, "e2e-cli-flag", "u"); status != http.StatusNotFound {
		t.Fatalf("evaluate after CLI delete: status = %d, want 404", status)
	}
	if status, raw := requestJSON(t, admin, http.MethodDelete, flagURL, ""); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("delete flag: status = %d, body = %s", status, raw)
	}
	if status, _ := evaluate(readToken, "e2e-new-checkout", "user-1"); status != http.StatusNotFound {
		t.Fatalf("evaluate after delete: status = %d, want 404", status)
	}
	if status, raw := requestJSON(t, admin, http.MethodGet, flagsURL(e2eFlagsAppA), ""); status != http.StatusOK || strings.Contains(string(raw), "e2e-new-checkout") {
		t.Fatalf("list after delete: status = %d, body = %s", status, raw)
	}

	// The generic audit log records the changes.
	if status, raw := requestJSON(t, admin, http.MethodGet, ts.URL+"/api/v1/audit-log?limit=100", ""); status != http.StatusOK || !strings.Contains(string(raw), "/flags") {
		t.Fatalf("audit log: status = %d, want entries for the flag routes, body = %.300s", status, raw)
	}
}

func createE2EToken(t *testing.T, admin *http.Client, baseURL, name, abilities string) string {
	t.Helper()
	status, body := postJSON(t, admin, baseURL+"/api/v1/auth/tokens", `{"name":"`+name+`","abilities":`+abilities+`}`)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create token %s: status = %d, body = %s", name, status, body)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || out.Token == "" {
		t.Fatalf("create token %s: decode %s: %v", name, body, err)
	}
	return out.Token
}

func buildE2ECLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "e2e-cli")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/GLINCKER/levelrail/cmd/levelrail-cli") //nolint:gosec,noctx // fixed arguments, one-shot build in a test
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build the CLI in this environment: %v\n%s", err, out)
	}
	return bin
}

func newE2EMCPSession(t *testing.T, baseURL, token string) *mcp.ClientSession {
	t.Helper()
	server := mcptools.NewServer(apiclient.NewClient(baseURL, token))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	go func() { _ = server.Run(ctx, serverTransport) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "e2e-client", Version: "0.0.0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("mcp connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callE2EMCP(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) []byte {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("mcp %s: err = %v, result = %+v", tool, err, result)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("mcp %s: marshal structured content: %v", tool, err)
	}
	return raw
}
