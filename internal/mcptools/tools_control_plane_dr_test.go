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

func TestControlPlaneDRTools(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/system/control-plane-dr":
			_ = json.NewEncoder(w).Encode(apiclient.ControlPlaneDR{
				Enabled: true, Recipients: []string{"age1pub"}, DrillSchedule: "0 5 * * 0",
				LastDrill: apiclient.ControlPlaneDRDrill{OK: true, Partial: true, Detail: "partial drill"},
				Warnings:  []apiclient.ControlPlaneDRWarning{{Code: "drill_partial", Message: "m"}},
			})
		case "/api/v1/system/control-plane-dr/backups":
			_ = json.NewEncoder(w).Encode([]apiclient.ControlPlaneOffboxBackup{{Key: "cp-backups/i/2026/09/25/x.db.age", Complete: true, SizeBytes: 9}})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	ctx := context.Background()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_control_plane_dr_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var st apiclient.ControlPlaneDR
	decodeStructured(t, res, &st)
	if !st.Enabled || len(st.Warnings) != 1 || st.Recipients[0] != "age1pub" {
		t.Errorf("status = %+v", st)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "list_control_plane_offbox_backups", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var list controlPlaneOffboxBackupsOutput
	decodeStructured(t, res, &list)
	if len(list.Backups) != 1 || !list.Backups[0].Complete {
		t.Errorf("backups = %+v", list.Backups)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "get_control_plane_drill_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var drill controlPlaneDrillOutput
	decodeStructured(t, res, &drill)
	if !drill.LastDrill.Partial || drill.DrillSchedule != "0 5 * * 0" {
		t.Errorf("drill = %+v", drill)
	}
	raw, _ := json.Marshal(drill)
	if strings.Contains(string(raw), "AGE-SECRET-KEY") {
		t.Error("drill output carries key material")
	}
}
