package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listToolsAgainst(t *testing.T, srvURL string) map[string]bool {
	t.Helper()
	server, _ := NewServerWithOptions(apiclient.NewClient(srvURL, "t"), Options{Mode: ModeFull})
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(context.Background(), st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	names := map[string]bool{}
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names[tool.Name] = true
	}
	return names
}

func TestAIControlHidesToolsByMode(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       any
		wantDeploy bool
		wantList   bool
		wantTotal1 bool
	}{
		{"off by 403", http.StatusForbidden, map[string]string{"error": "agent access is disabled by an administrator"}, false, false, true},
		{"observe", 200, map[string]string{"mode": "observe"}, false, true, false},
		{"operate", 200, map[string]string{"mode": "operate"}, true, true, false},
		{"admin", 200, map[string]string{"mode": "admin"}, true, true, false},
		{"unreadable mode leaves list alone", http.StatusInternalServerError, map[string]string{"error": "x"}, true, true, false},
		{"unrelated 403 leaves list alone", http.StatusForbidden, map[string]string{"error": "token lacks the required ability"}, true, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer srv.Close()
			names := listToolsAgainst(t, srv.URL)
			if !names[aiControlStatusTool] {
				t.Error("ai_control_status must always be listed")
			}
			if names["deploy_app"] != tc.wantDeploy {
				t.Errorf("deploy_app listed = %v, want %v", names["deploy_app"], tc.wantDeploy)
			}
			if names["list_apps"] != tc.wantList {
				t.Errorf("list_apps listed = %v, want %v", names["list_apps"], tc.wantList)
			}
			if tc.wantTotal1 && len(names) != 1 {
				t.Errorf("off mode lists %d tools, want 1", len(names))
			}
		})
	}
}
