package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSupplyChainTools(t *testing.T) {
	two := 2
	var paths []string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/web/deploy-attempts":
			_ = json.NewEncoder(w).Encode([]apiclient.DeployAttemptResource{
				{ID: "da_plain"},
				{ID: "da_sbom", SBOMPackages: &two},
			})
		case "/api/v1/apps/web/deployments/da_sbom/sbom":
			_ = json.NewEncoder(w).Encode(apiclient.SBOMSummary{DeploymentID: "da_sbom", PackageCount: 2, Format: "spdx"})
		case "/api/v1/apps/web/deployments/da_sbom/vulnerabilities":
			_ = json.NewEncoder(w).Encode(apiclient.VulnReport{DeploymentID: "da_sbom"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_deploy_sbom", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatal(err)
	}
	var sbom apiclient.SBOMSummary
	decodeStructured(t, result, &sbom)
	if sbom.DeploymentID != "da_sbom" || sbom.PackageCount != 2 {
		t.Errorf("sbom = %+v", sbom)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_deploy_vulnerabilities", Arguments: map[string]any{"name": "web", "deploy_id": "da_sbom"}})
	if err != nil {
		t.Fatal(err)
	}
	var rep apiclient.VulnReport
	decodeStructured(t, result, &rep)
	if rep.DeploymentID != "da_sbom" {
		t.Errorf("report = %+v", rep)
	}
	for _, p := range paths {
		if p[:3] != "GET" {
			t.Errorf("supply chain tools are read only, saw %s", p)
		}
	}
}

func TestSupplyChainTools_NoDataYet(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.DeployAttemptResource{{ID: "da_plain"}})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_deploy_sbom", Arguments: map[string]any{"name": "web"}})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected a tool error when no deploy has an SBOM")
	}
}
