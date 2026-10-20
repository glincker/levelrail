package mcptools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type recordedCall struct {
	method, path, body string
}

type callRecorder struct {
	mu    sync.Mutex
	calls []recordedCall
}

func (r *callRecorder) record(req *http.Request) {
	b, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCall{req.Method, req.URL.Path, string(b)})
}

func (r *callRecorder) snapshot() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

// safeDryRunCall reports whether a recorded call is a read or an API side dry run.
func safeDryRunCall(c recordedCall) bool {
	switch {
	case c.method == http.MethodGet:
		return true
	case c.method == http.MethodPost && c.path == "/api/v1/apps/bulk":
		return strings.Contains(c.body, `"dry_run":true`)
	case c.method == http.MethodPost && c.path == "/api/v1/apply/plan":
		return true
	}
	return false
}

func planTestSession(t *testing.T, rec *callRecorder, app apiclient.AppResource, freeze apiclient.DeployFreezeResource) *mcp.ClientSession {
	t.Helper()
	return newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/deploy-freeze"):
			_ = json.NewEncoder(w).Encode(freeze)
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/web/deploys/"):
			_ = json.NewEncoder(w).Encode(apiclient.DeployAttemptResource{ID: "dep_1", ServiceName: "web", Status: "running"})
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/") && strings.Count(r.URL.Path, "/") == 4:
			_ = json.NewEncoder(w).Encode(app)
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})
}

func callPlan(t *testing.T, s *mcp.ClientSession, tool string, args map[string]any) PlanResult {
	t.Helper()
	callArgs := map[string]any{"tool": tool}
	if args != nil {
		callArgs["arguments"] = args
	}
	result, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "plan_change", Arguments: callArgs})
	if err != nil {
		t.Fatal(err)
	}
	var got PlanResult
	decodeStructured(t, result, &got)
	return got
}

func TestPlanChange(t *testing.T) {
	until := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	frozen := apiclient.DeployFreezeResource{}
	frozen.Status.Frozen, frozen.Status.Reason, frozen.Status.Until = true, "release freeze", &until
	app := apiclient.AppResource{Name: "web", Image: "acme/web:1", ImageDigest: "sha256:aaa", Domains: []string{"a.example.com"}, Env: map[string]string{"LOG": "info"}, SecretEnv: []string{"API_KEY"}, EnvironmentID: "env_prod"}

	tests := []struct {
		name        string
		freeze      apiclient.DeployFreezeResource
		tool        string
		args        map[string]any
		wantChanges []PlanChange
		wantBlocked bool
		wantSummary string
	}{
		{"deploy shows image change and digest", apiclient.DeployFreezeResource{}, "deploy_app", map[string]any{"name": "web", "image": "acme/web:2"},
			[]PlanChange{{"image", "acme/web:1", "acme/web:2"}, {"image_digest", "sha256:aaa", "resolved when the deploy runs"}}, false, ""},
		{"deploy pinned digest", apiclient.DeployFreezeResource{}, "deploy_app", map[string]any{"name": "web", "image": "acme/web@sha256:bbb"},
			[]PlanChange{{"image", "acme/web:1", "acme/web@sha256:bbb"}, {"image_digest", "sha256:aaa", "sha256:bbb"}}, false, ""},
		{"deploy blocked by a freeze window", frozen, "deploy_app", map[string]any{"name": "web", "image": "acme/web:2"}, nil, true, ""},
		{"rollback uses the same plan", frozen, "rollback_app", map[string]any{"name": "web", "image": "acme/web:0"}, nil, true, ""},
		{"set env new key", apiclient.DeployFreezeResource{}, "set_app_env", map[string]any{"name": "web", "key": "NEW", "value": "s3cret"},
			[]PlanChange{{"env.NEW", "unset", "set (new value hidden)"}}, false, ""},
		{"set env unchanged", apiclient.DeployFreezeResource{}, "set_app_env", map[string]any{"name": "web", "key": "LOG", "value": "info"}, nil, false, "no change"},
		{"set secret", apiclient.DeployFreezeResource{}, "set_app_env", map[string]any{"name": "web", "key": "API_KEY", "value": "x", "secret": true},
			[]PlanChange{{"secret.API_KEY", "set (value hidden)", "set (new value hidden)"}}, false, ""},
		{"unset env", apiclient.DeployFreezeResource{}, "unset_app_env", map[string]any{"name": "web", "key": "LOG"},
			[]PlanChange{{"env.LOG", "set (value hidden)", "unset"}}, false, ""},
		{"unset missing env", apiclient.DeployFreezeResource{}, "unset_app_env", map[string]any{"name": "web", "key": "NOPE"}, nil, false, "no change"},
		{"domains diff", apiclient.DeployFreezeResource{}, "set_app_domains", map[string]any{"name": "web", "domains": []string{"a.example.com", "b.example.com"}},
			[]PlanChange{{"domains", "a.example.com", "a.example.com,b.example.com"}}, false, ""},
		{"domains same set", apiclient.DeployFreezeResource{}, "set_app_domains", map[string]any{"name": "web", "domains": []string{"a.example.com"}}, nil, false, "no change"},
		{"cancel a running deploy", apiclient.DeployFreezeResource{}, "cancel_deploy", map[string]any{"name": "web", "deploy_id": "dep_1"},
			[]PlanChange{{"deploy.status", "running", "canceled"}}, false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &callRecorder{}
			session := planTestSession(t, rec, app, tc.freeze)
			got := callPlan(t, session, tc.tool, tc.args)
			if !got.Plannable || !got.DryRun || got.Executed {
				t.Fatalf("flags wrong: %+v", got)
			}
			if got.Blocked != tc.wantBlocked {
				t.Errorf("blocked = %v, want %v (%v)", got.Blocked, tc.wantBlocked, got.Blockers)
			}
			if tc.wantChanges != nil && !slices.Equal(got.Changes, tc.wantChanges) {
				t.Errorf("changes = %+v, want %+v", got.Changes, tc.wantChanges)
			}
			if tc.wantSummary != "" && !strings.Contains(got.Summary, tc.wantSummary) {
				t.Errorf("summary = %q, want it to contain %q", got.Summary, tc.wantSummary)
			}
			for _, c := range rec.snapshot() {
				if !safeDryRunCall(c) {
					t.Errorf("plan made a mutating call: %s %s", c.method, c.path)
				}
			}
			if strings.Contains(strings.Join(got.Blockers, ""), "s3cret") || strings.Contains(mustJSON(t, got), "s3cret") {
				t.Errorf("plan leaked a value: %s", mustJSON(t, got))
			}
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPlanChangeNeverMutates plans every mutating tool and proves that only
// reads and API side dry runs reach the control plane; tools without a
// planner say so and make no call at all.
func TestPlanChangeNeverMutates(t *testing.T) {
	args := map[string]any{"name": "web", "key": "K", "value": "v", "image": "img:1", "deploy_id": "dep_1", "domains": []string{"x.example.com"}, "to": "env_2", "action": "restart", "names": []string{"web"}, "files": []map[string]any{{"name": "a.yaml", "content": "x"}}}
	var mutating []string
	for name, meta := range toolTable {
		if meta.Class == ClassMutate || meta.Class == ClassDestructive {
			mutating = append(mutating, name)
		}
	}
	slices.Sort(mutating)
	if len(mutating) < 30 {
		t.Fatalf("only %d mutating tools found, the classification table changed shape", len(mutating))
	}
	for _, tool := range mutating {
		t.Run(tool, func(t *testing.T) {
			rec := &callRecorder{}
			session := planTestSession(t, rec, apiclient.AppResource{Name: "web", Image: "img:0"}, apiclient.DeployFreezeResource{})
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "plan_change", Arguments: map[string]any{"tool": tool, "arguments": args}})
			if err != nil {
				t.Fatal(err)
			}
			_, planned := planners[tool]
			if !planned {
				var got PlanResult
				decodeStructured(t, result, &got)
				if got.Plannable || got.Executed || !strings.Contains(got.Summary, "cannot be planned") {
					t.Errorf("unplannable tool must say so: %+v", got)
				}
				if n := len(rec.snapshot()); n != 0 {
					t.Errorf("unplannable tool made %d API calls", n)
				}
				return
			}
			for _, c := range rec.snapshot() {
				if !safeDryRunCall(c) {
					t.Errorf("planning %s made a mutating call: %s %s %s", tool, c.method, c.path, c.body)
				}
			}
		})
	}
}

func TestDryRunArgumentOnMutatingToolIsRejectedNotExecuted(t *testing.T) {
	rec := &callRecorder{}
	session := planTestSession(t, rec, apiclient.AppResource{Name: "web"}, apiclient.DeployFreezeResource{})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "deploy_app", Arguments: map[string]any{"name": "web", "image": "img:2", "dry_run": true}})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatalf("dry_run on deploy_app was accepted: %+v", result)
	}
	if n := len(rec.snapshot()); n != 0 {
		t.Errorf("rejected call still reached the API %d times: %+v", n, rec.snapshot())
	}
}

func TestPlanChangeReadToolAndUnknown(t *testing.T) {
	rec := &callRecorder{}
	session := planTestSession(t, rec, apiclient.AppResource{}, apiclient.DeployFreezeResource{})
	got := callPlan(t, session, "list_apps", nil)
	if got.Plannable || !strings.Contains(got.Summary, "read-only") {
		t.Errorf("read tool: %+v", got)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "plan_change", Arguments: map[string]any{"tool": "no_such_tool"}})
	if err == nil && !result.IsError {
		t.Error("unknown tool must be an error")
	}
	if n := len(rec.snapshot()); n != 0 {
		t.Errorf("made %d API calls", n)
	}
}

func TestPlannersAreAllMutatingTools(t *testing.T) {
	for _, name := range PlannableTools() {
		meta, ok := toolTable[name]
		if !ok || meta.Class == ClassRead {
			t.Errorf("planner %q is not a classified mutating tool", name)
		}
	}
}
