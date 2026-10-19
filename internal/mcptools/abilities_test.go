package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func abilityServer(t *testing.T, abilities []string, policies int) *apiclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/my-access" {
			http.NotFound(w, r)
			return
		}
		res := apiclient.MyAccessResource{Kind: "token", Abilities: abilities, Policies: make([]apiclient.MyAccessPolicy, policies)}
		_ = json.NewEncoder(w).Encode(res)
	}))
	t.Cleanup(srv.Close)
	return apiclient.NewClient(srv.URL, "t")
}

func connectWith(t *testing.T, client *apiclient.Client, opts Options) *mcp.ClientSession {
	t.Helper()
	server, _ := NewServerWithOptions(client, opts)
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(context.Background(), st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func listWith(t *testing.T, session *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	var tools []*mcp.Tool
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}

func toolNameSet(tools []*mcp.Tool) map[string]bool {
	out := map[string]bool{}
	for _, tool := range tools {
		out[tool.Name] = true
	}
	return out
}

func TestToolListFollowsTokenAbilities(t *testing.T) {
	tokens := []struct {
		name      string
		abilities []string
		policies  int
		want      []string
		hidden    []string
	}{
		{"read only", []string{"read"}, 0, []string{"list_apps", "get_app_env", "whoami"}, []string{"set_app_env", "set_app_domains", "deploy_app"}},
		{"read and deploy", []string{"read", "deploy"}, 0, []string{"list_apps", "deploy_app", "restart_app"}, []string{"set_app_env", "set_app_domains", "unset_app_env"}},
		{"write sensitive", []string{"read", "write", "write:sensitive"}, 0, []string{"list_apps", "set_app_env", "set_app_domains"}, []string{"deploy_app", "restart_app"}},
		{"root", []string{"root"}, 0, []string{"list_apps", "set_app_env", "deploy_app"}, nil},
		{"policies keep the full list", []string{"read"}, 1, []string{"list_apps", "set_app_env", "deploy_app"}, nil},
	}
	for _, mode := range []Mode{ModeStandard, ModeFull} {
		unfiltered := toolNameSet(listAll(t, Options{Mode: mode}))
		for _, tc := range tokens {
			t.Run(string(mode)+"/"+tc.name, func(t *testing.T) {
				got := toolNameSet(listWith(t, connectWith(t, abilityServer(t, tc.abilities, tc.policies), Options{Mode: mode})))
				for _, n := range tc.want {
					if !got[n] {
						t.Errorf("%s missing from the list", n)
					}
				}
				for _, n := range tc.hidden {
					if got[n] {
						t.Errorf("%s must be hidden", n)
					}
				}
				for n := range got {
					if !unfiltered[n] {
						t.Errorf("%s appeared though the mode does not list it", n)
					}
					m := toolTable[n]
					if !abilityAllows(tc.abilities, n, m) && tc.policies == 0 {
						t.Errorf("%s is listed but the token cannot call it", n)
					}
				}
				if tc.abilities[0] == "root" && len(got) != len(unfiltered) {
					t.Errorf("root sees %d tools, mode lists %d", len(got), len(unfiltered))
				}
			})
		}
	}
}

func TestReadTokenNeverSeesWriteTools(t *testing.T) {
	for _, mode := range []Mode{ModeStandard, ModeFull} {
		for _, tool := range listWith(t, connectWith(t, abilityServer(t, []string{"read"}, 0), Options{Mode: mode})) {
			if m := toolTable[tool.Name]; m.Class != ClassRead && !isToolsetMetaTool(tool.Name) {
				t.Errorf("mode %s offers %s (%s) to a read token", mode, tool.Name, m.Class)
			}
		}
	}
}

func TestEnableToolsetRespectsAbilities(t *testing.T) {
	session := connectWith(t, abilityServer(t, []string{"read"}, 0), Options{Mode: ModeFull, Dynamic: true})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "enable_toolset", Arguments: map[string]any{"toolsets": []string{"deploys"}}})
	if err != nil || res.IsError {
		t.Fatalf("enable deploys: %v %+v", err, res)
	}
	got := toolNameSet(listWith(t, session))
	if !got["list_deploys"] {
		t.Error("read tools of an enabled toolset must be listed")
	}
	for _, n := range []string{"rollback_app", "cancel_deploy", "promote_app"} {
		if got[n] {
			t.Errorf("%s must stay hidden from a read token", n)
		}
	}
}

func TestForbiddenHint(t *testing.T) {
	deny := &apiclient.APIError{StatusCode: http.StatusForbidden, Message: "your account lacks the required ability (needs write:sensitive)"}
	got := forbiddenHint("set_app_env", deny)
	if got == nil || !strings.Contains(got.Error(), `"missing_ability":"write:sensitive"`) || !strings.Contains(got.Error(), `"use_instead":"plan_change"`) {
		t.Fatalf("hint = %v", got)
	}
	if n := len(got.Error()) / 4; n > 40 {
		t.Errorf("hint is ~%d tokens, want at most 40", n)
	}
	other := &apiclient.APIError{StatusCode: http.StatusForbidden, Message: "agent access is disabled"}
	if got := forbiddenHint("set_app_env", other); !errors.Is(got, other) {
		t.Errorf("policy refusals must pass through unchanged, got %v", got)
	}
}

func TestAbilityTokenCounts(t *testing.T) {
	for _, mode := range []Mode{ModeStandard, ModeFull} {
		for _, abilities := range [][]string{{"read"}, {"read", "deploy"}, {"read", "write", "write:sensitive"}, {"root"}} {
			for _, dynamic := range []bool{false, true} {
				tools := listWith(t, connectWith(t, abilityServer(t, abilities, 0), Options{Mode: mode, Dynamic: dynamic, OmitOutputSchemas: true}))
				t.Logf("mode=%s dynamic=%v abilities=%v: %d tools ~%d tokens", mode, dynamic, abilities, len(tools), estimateTokens(t, tools))
			}
		}
	}
}
