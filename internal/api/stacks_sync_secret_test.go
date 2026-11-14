package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DeleteAll lets the fake stand in for the sealed manager on stack delete.
func (f *fakeGitSourceSecrets) DeleteAll(_ context.Context, serviceName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, serviceName)
	return nil
}

func (e *syncHookEnv) getSync(t *testing.T, stack string) (bool, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.root, http.MethodGet, "/api/v1/stacks/"+stack+"/sync", ""))
	var out struct {
		Set bool `json:"webhook_secret_set"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("get sync = %d %s", rec.Code, rec.Body.String())
	}
	return out.Set, rec.Body.String()
}

func TestStackSyncSecret_FlagAndCleanupOnDelete(t *testing.T) {
	env := newSyncHookEnv(t)
	if set, body := env.getSync(t, "infra"); !set || strings.Contains(body, env.secret) {
		t.Fatalf("after rotate: set=%v body=%s", set, body)
	}
	// A stack that never rotated reports false.
	if _, err := env.db.CreateStack(t.Context(), store.StackRecord{ID: "stk-new", Name: "fresh", Scope: store.StackScopeInstall, DriftPolicy: "warn", CreatedBy: "t"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if set, _ := env.getSync(t, "fresh"); set {
		t.Fatal("webhook_secret_set true before any rotate")
	}
	rec := httptest.NewRecorder()
	env.rt.Handler().ServeHTTP(rec, authedRequest(t, env.root, http.MethodDelete, "/api/v1/stacks/infra", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	has := func(id string) bool {
		ok, _ := env.rt.gitSourceSecrets.Exists(t.Context(), store.StackSyncSecretsKey(id), stackSyncWebhookSecretKey)
		return ok
	}
	if has(env.stackID) {
		t.Fatal("sealed secret survived the stack delete")
	}
	other, err := env.db.GetStackByName(t.Context(), "dormant")
	if err != nil {
		t.Fatal(err)
	}
	if !has(other.ID) {
		t.Fatal("delete removed another stack's secret")
	}
}
