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

func TestGetAttention(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps":
			_, _ = w.Write([]byte(`[{"name":"web","status":{"label":"Crashlooping","variant":"destructive"}},{"name":"ok","status":{"label":"Running","variant":"success"}}]`))
		case "/api/v1/nodes":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/certificates":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/system/doctor":
			_, _ = w.Write([]byte(`{"ok":false,"checks":[{"code":"disk","name":"Disk","status":"warn","message":"low space"}]}`))
		case "/api/v1/deploys/failed":
			_, _ = w.Write([]byte(`[{"service_name":"api","error":"build failed"}]`))
		case "/api/v1/system/status":
			_, _ = w.Write([]byte(`{"data_dir_total_bytes":100,"data_dir_free_bytes":3}`))
		case "/api/v1/apps/api/diagnose", "/api/v1/apps/web/diagnose":
			_, _ = w.Write([]byte(`{"confidence":"none","fixable":false}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_attention", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}
	var out attentionOutput
	decodeStructured(t, result, &out)
	kinds := []string{}
	for _, it := range out.Items {
		kinds = append(kinds, it.Severity+":"+it.Kind)
	}
	want := []string{"critical:disk", "critical:deploy", "critical:app", "warning:doctor"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("items = %v, want %v", kinds, want)
	}
}

func TestGetAttention_Error(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_attention", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("transport error = %v", err)
	}
	if !result.IsError || !strings.Contains(toolResultText(result), "list apps") {
		t.Errorf("result = %q, want an error mentioning list apps", toolResultText(result))
	}
}

func TestGetNodeStatusHistory(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes/n1/events" || r.URL.Query().Get("limit") != "5" {
			t.Errorf("request = %q, want /api/v1/nodes/n1/events?limit=5", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"from_status":"online","to_status":"offline","created_at":"2026-09-23T10:00:00Z"}]`))
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_node_status_history", Arguments: map[string]any{"id": "n1", "limit": 5}})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}
	var events []apiclient.NodeStatusEventResource
	decodeStructured(t, result, &events)
	if len(events) != 1 || events[0].ToStatus != "offline" {
		t.Errorf("events = %+v", events)
	}
}

func TestListAuditLog_SearchAndFailed(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("q") != "token" || q.Get("status") != "failed" {
			t.Errorf("query = %q, want q=token and status=failed", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_audit_log", Arguments: map[string]any{"q": "token", "status": "failed"}})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}
}

func TestControlPlaneBackups(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/backups" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		b := apiclient.ControlPlaneBackup{Name: "cp-1.db", SizeBytes: 10, SHA256: "abc"}
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(b)
			return
		}
		_ = json.NewEncoder(w).Encode([]apiclient.ControlPlaneBackup{b})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_control_plane_backups", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	var list controlPlaneBackupsOutput
	decodeStructured(t, result, &list)
	if len(list.Backups) != 1 || list.Backups[0].Name != "cp-1.db" {
		t.Errorf("backups = %+v", list.Backups)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_control_plane_backup", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("create error = %v", err)
	}
	var created apiclient.ControlPlaneBackup
	decodeStructured(t, result, &created)
	if created.SHA256 != "abc" {
		t.Errorf("created = %+v", created)
	}
}

func TestVerifyControlPlaneBackupAndFailedDeploysTools(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/backups/snap-1.db/verify":
			_, _ = w.Write([]byte(`{"name":"snap-1.db","ok":false,"checks":[{"name":"integrity","ok":false,"detail":"bad page"}],"verified_at":"2026-09-24T00:00:00Z"}`))
		case r.URL.Path == "/api/v1/deploys/failed":
			if r.URL.Query().Get("since") != "6h" {
				t.Errorf("since = %q, want 6h", r.URL.Query().Get("since"))
			}
			_, _ = w.Write([]byte(`[{"service_name":"api","error":"build failed","last_good_image":"img:1"}]`))
		default:
			t.Errorf("unexpected %s %q", r.Method, r.URL.Path)
		}
	})

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "verify_control_plane_backup", Arguments: map[string]any{"name": "snap-1.db"}})
	if err != nil {
		t.Fatalf("verify CallTool error = %v", err)
	}
	var ver apiclient.ControlPlaneBackupVerification
	decodeStructured(t, res, &ver)
	if ver.OK || len(ver.Checks) != 1 || ver.Checks[0].Detail != "bad page" {
		t.Fatalf("verification = %+v", ver)
	}

	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_failed_deploys", Arguments: map[string]any{"since": "6h"}})
	if err != nil {
		t.Fatalf("failed deploys CallTool error = %v", err)
	}
	var out failedDeploysOutput
	decodeStructured(t, res, &out)
	if len(out.Deploys) != 1 || out.Deploys[0].LastGoodImage != "img:1" {
		t.Fatalf("deploys = %+v", out.Deploys)
	}
}
