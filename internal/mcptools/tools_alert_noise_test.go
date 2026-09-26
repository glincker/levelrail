package mcptools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSilenceAlertRuleTool(t *testing.T) {
	var gotPath, gotBody string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.Path, string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.SilenceResource{ID: "sil_1", Status: "active"})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "silence_alert_rule",
		Arguments: map[string]any{"app": "web", "rule_id": "r1", "duration": "4h", "reason": "noisy"}})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var got apiclient.SilenceResource
	decodeStructured(t, result, &got)
	if got.ID != "sil_1" || gotPath != "/api/v1/apps/web/alerts/r1/silence" {
		t.Errorf("got = %+v path = %q", got, gotPath)
	}
	var sent apiclient.CreateSilenceRequest
	_ = json.Unmarshal([]byte(gotBody), &sent)
	if sent.Duration != "4h" || sent.Reason != "noisy" {
		t.Errorf("request = %+v", sent)
	}
}

func TestCreateAndExpireSilenceTools(t *testing.T) {
	var paths []string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.SilenceResource{ID: "sil_2"})
	})
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_alert_silence",
		Arguments: map[string]any{"apps": []string{"web"}, "duration": "1h"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "expire_alert_silence", Arguments: map[string]any{"id": "sil_2"}}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "POST /api/v1/alert-silences" || paths[1] != "DELETE /api/v1/alert-silences/sil_2" {
		t.Errorf("paths = %v", paths)
	}
}

func TestAlertHistoryAndStatusPageTools(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/alert-history":
			if r.URL.Query().Get("outcome") != "silenced" {
				t.Errorf("query = %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode([]apiclient.AlertHistoryEntry{{RuleName: "cpu", Outcome: "silenced"}})
		case "/api/v1/status-page":
			_ = json.NewEncoder(w).Encode(apiclient.StatusPageSettings{Enabled: true, Title: "Acme"})
		case "/api/v1/status-page/preview":
			_ = json.NewEncoder(w).Encode(apiclient.StatusPageView{Title: "Acme", Status: "operational"})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_alert_history", Arguments: map[string]any{"outcome": "silenced"}})
	if err != nil {
		t.Fatal(err)
	}
	var hist []apiclient.AlertHistoryEntry
	decodeStructured(t, result, &hist)
	if len(hist) != 1 || hist[0].Outcome != "silenced" {
		t.Errorf("history = %+v", hist)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_status_page", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var page statusPageOutput
	decodeStructured(t, result, &page)
	if !page.Settings.Enabled || page.Preview.Status != "operational" {
		t.Errorf("page = %+v", page)
	}
}
