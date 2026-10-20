package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResourceAdvice(t *testing.T) {
	var gotURI string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AdvisorFindings{Findings: []apiclient.AdvisorFinding{{
			Finding: resourceadvisor.Finding{ID: "memory_limit_missing:database:main", Kind: resourceadvisor.KindMemoryLimitMissing, Severity: resourceadvisor.SeveritySuggest},
		}}})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      resourceAdviceTool,
		Arguments: map[string]any{"resource": "database:main", "severity": "suggest"},
	})
	if err != nil {
		t.Fatalf("CallTool(resource_advice) error = %v", err)
	}
	if gotURI != "/api/v1/resource-advisor/findings?resource=database%3Amain&severity=suggest" {
		t.Errorf("uri = %q", gotURI)
	}
	var out apiclient.AdvisorFindings
	decodeStructured(t, result, &out)
	if len(out.Findings) != 1 || out.Findings[0].Kind != resourceadvisor.KindMemoryLimitMissing {
		t.Errorf("findings = %+v", out.Findings)
	}
	if m := toolTable[resourceAdviceTool]; m.Class != ClassRead {
		t.Errorf("resource_advice must be a read tool, got %v", m.Class)
	}
}
