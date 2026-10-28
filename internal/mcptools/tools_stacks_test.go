package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestApplyStackNeedsConfirmAndSendsNoRequest(t *testing.T) {
	var calls atomic.Int32
	session := newTestSession(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	args := map[string]any{"name": "shop-prod", "files": []map[string]string{{"name": "a.yaml", "content": "x"}}}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_stack", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || calls.Load() != 0 {
		t.Fatalf("isError=%v calls=%d, want a local refusal", result.IsError, calls.Load())
	}
}

func TestApplyStackReturnsSummaryWithoutFieldValues(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/stacks/shop-prod/apply" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stack":"shop-prod","revision":{"number":2,"status":"applied"},` +
			`"result":{"applied":1,"failed":0,"plan":{"hash":"h1","summary":{"create":0,"update":1,"delete":0,"noop":0,"error":0},` +
			`"changes":[{"kind":"App","name":"web","action":"update","fields":[{"path":"env.X","new":"hunter2"}]}]}}}`))
	})
	args := map[string]any{"name": "shop-prod", "confirm": true, "files": []map[string]string{{"name": "a.yaml", "content": "x"}}}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_stack", Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("call = %v %+v", err, result)
	}
	raw, _ := json.Marshal(result.StructuredContent)
	if strings.Contains(string(raw), "hunter2") {
		t.Fatalf("output leaks a field value: %s", raw)
	}
	var got stackRunOutput
	decodeStructured(t, result, &got)
	if got.Revision != 2 || got.Applied != 1 || len(got.Changes) != 1 || got.Summary.Update != 1 {
		t.Fatalf("got %+v", got)
	}
}
