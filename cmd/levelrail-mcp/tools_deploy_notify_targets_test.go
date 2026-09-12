package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListDeployNotifyTargets(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/deploy-notify-targets" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/deploy-notify-targets", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.DeployNotifyTargetResource{
			{ID: "dnt_1", ChannelID: "ch_1", NotifyKind: "slack", Enabled: true},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_deploy_notify_targets",
		Arguments: map[string]any{"name": "web"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_deploy_notify_targets) error = %v", err)
	}
	var targets []apiclient.DeployNotifyTargetResource
	decodeStructured(t, result, &targets)
	if len(targets) != 1 || targets[0].ChannelID != "ch_1" {
		t.Errorf("targets = %+v, want one target on channel ch_1", targets)
	}
}

func TestListDeployNotifyTargets_Empty(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.DeployNotifyTargetResource{})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_deploy_notify_targets",
		Arguments: map[string]any{"name": "web"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_deploy_notify_targets) error = %v", err)
	}
	var targets []apiclient.DeployNotifyTargetResource
	decodeStructured(t, result, &targets)
	if len(targets) != 0 {
		t.Errorf("targets = %+v, want empty", targets)
	}
}
