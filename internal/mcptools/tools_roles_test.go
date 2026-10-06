package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListRoles(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/roles" {
			t.Errorf("request = %s %s, want GET /api/v1/roles", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.RoleResource{{ID: "role_guest", Name: "guest", Abilities: []string{"read"}, Visibility: "granted", Builtin: true}})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_roles"})
	if err != nil {
		t.Fatalf("CallTool(list_roles) error = %v", err)
	}
	var got []apiclient.RoleResource
	decodeStructured(t, result, &got)
	if len(got) != 1 || got[0].Visibility != "granted" {
		t.Errorf("roles = %+v", got)
	}
}
