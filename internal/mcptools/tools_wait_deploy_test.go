package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func waitTestSession(t *testing.T, outcomes []string) (*mcp.ClientSession, *[]string) {
	t.Helper()
	t.Setenv(EnvWaitInterval, "1")
	var mu sync.Mutex
	var paths []string
	calls := 0
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, r.URL.Path)
		i := min(calls, len(outcomes)-1)
		calls++
		d := apiclient.DeployAttemptResource{ID: "dep_9", ServiceName: "web", Status: "running", Outcome: outcomes[i]}
		if outcomes[i] == "failed" {
			d.Status = "failed"
			d.Failure = &apiclient.DeployFailure{Code: "missing_env", DeployID: "dep_9", App: "web"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d)
	})
	return session, &paths
}

func TestWaitForDeployPollsUntilFailure(t *testing.T) {
	session, paths := waitTestSession(t, []string{"in_progress", "failed"})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "wait_for_deploy", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatal(err)
	}
	var got waitForDeployOutput
	decodeStructured(t, result, &got)
	if got.Status != "failed" || got.PollAgain || got.Failure == nil || got.Failure.Code != "missing_env" || got.DeployID != "dep_9" {
		t.Fatalf("got %+v", got)
	}
	if len(*paths) != 2 || (*paths)[0] != "/api/v1/apps/web/deploys/latest" || (*paths)[1] != "/api/v1/apps/web/deploys/dep_9" {
		t.Errorf("latest was not pinned to the concrete id: %v", *paths)
	}
}

func TestWaitForDeployWindowReturnsPollAgain(t *testing.T) {
	session, _ := waitTestSession(t, []string{"in_progress"})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "wait_for_deploy", Arguments: map[string]any{"name": "web", "max_wait_seconds": 1}})
	if err != nil {
		t.Fatal(err)
	}
	var got waitForDeployOutput
	decodeStructured(t, result, &got)
	if got.Status != "in_progress" || !got.PollAgain || got.DeployID != "dep_9" {
		t.Fatalf("got %+v", got)
	}
}
