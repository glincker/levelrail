package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListPolicyTemplatesTool(t *testing.T) {
	var gotPath string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(apiclient.PolicyTemplateList{Version: 1, Templates: []apiclient.PolicyTemplate{
			{ID: "deployer-nonprod", Name: "Deployer", Document: apiclient.PolicyDocument{Statement: []apiclient.PolicyStatement{{Effect: "Allow", Action: []string{"read"}, Resource: []string{"*"}}}}},
		}})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_policy_templates"})
	if err != nil || res.IsError {
		t.Fatalf("CallTool err=%v isError=%v", err, res != nil && res.IsError)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if gotPath != "/api/v1/iam/policy-templates" || !strings.Contains(string(b), "deployer-nonprod") {
		t.Fatalf("path = %q, out = %s", gotPath, b)
	}
	meta, ok := Lookup("list_policy_templates")
	if !ok || meta.Class != ClassRead {
		t.Fatalf("meta = %+v ok = %v, want read class", meta, ok)
	}
}
