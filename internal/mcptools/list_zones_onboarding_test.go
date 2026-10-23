package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func callListZones(t *testing.T, args map[string]any) map[string]json.RawMessage {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/nodes/zones":
			_ = json.NewEncoder(w).Encode(apiclient.Zones{Zones: []apiclient.ZoneSummary{{Zone: "a"}}, Labeled: true})
		case "/api/v1/nodes/domains":
			_ = json.NewEncoder(w).Encode(apiclient.NodeDomains{Total: 2, Providers: []apiclient.NodeDomainGroup{{Name: "aws"}}})
		case "/api/v1/nodes/nd_1/onboarding":
			_ = json.NewEncoder(w).Encode(apiclient.NodeOnboarding{NodeID: "nd_1", Stage: "needs_attention", Summary: apiclient.NodePreflightSummary{Fail: 1},
				Checks: []apiclient.NodePreflightCheck{{ID: "docker", Status: "fail", Fix: "install docker"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	session := connectWith(t, apiclient.NewClient(srv.URL, "t"), Options{Mode: ModeFull})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_zones", Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("call: err=%v result=%+v", err, res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestListZonesKeepsItsOriginalShape(t *testing.T) {
	out := callListZones(t, map[string]any{})
	if _, ok := out["zones"]; !ok {
		t.Fatalf("zones missing: %v", out)
	}
	if _, ok := out["domains"]; ok {
		t.Error("capacity must be opt in")
	}
}

func TestListZonesCapacityAddsDomains(t *testing.T) {
	out := callListZones(t, map[string]any{"capacity": true})
	var d apiclient.NodeDomains
	if err := json.Unmarshal(out["domains"], &d); err != nil || d.Total != 2 {
		t.Fatalf("domains = %s err=%v", out["domains"], err)
	}
}

func TestListZonesNodeIDReturnsPreflightWithFixes(t *testing.T) {
	out := callListZones(t, map[string]any{"node_id": "nd_1"})
	var ob apiclient.NodeOnboarding
	if err := json.Unmarshal(out["onboarding"], &ob); err != nil || ob.Stage != "needs_attention" || ob.Checks[0].Fix == "" {
		t.Fatalf("onboarding = %s err=%v", out["onboarding"], err)
	}
}
