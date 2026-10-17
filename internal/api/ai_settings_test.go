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

func TestHandleGetAIAssistantSettings_Default(t *testing.T) {
	rt, db, _ := providerRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := aiDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/ai-assistant", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got aiAssistantSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Configured || got.Active != nil || got.Fallback == nil || len(got.Fallback) != 0 || got.Limits.MaxSteps == 0 {
		t.Errorf("got %+v, want unconfigured with an empty fallback list and limits", got)
	}
	for _, legacy := range []string{`"provider"`, `"model"`} {
		if strings.Contains(rec.Body.String(), legacy) {
			t.Errorf("response still carries the top-level %s field: %s", legacy, rec.Body.String())
		}
	}
}

func TestHandleAIAssistantSettings_NoVault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		body := ""
		if method == http.MethodPut {
			body = `{"fallback":[]}`
		}
		rec := aiDo(t, rt, cookie, method, "/api/v1/settings/ai-assistant", body)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s status = %d, want %d", method, rec.Code, http.StatusNotImplemented)
		}
	}
}

func TestHandleUpdateAIAssistantSettings_RequiresChoice(t *testing.T) {
	rt, db, _ := providerRouter(t)
	cookie := loginTestSession(t, rt, db)

	for _, body := range []string{`{}`, `not json`} {
		rec := aiDo(t, rt, cookie, http.MethodPut, "/api/v1/settings/ai-assistant", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleAIAssistantSettings_DeleteRouteRemoved(t *testing.T) {
	rt, db, _ := providerRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := aiDo(t, rt, cookie, http.MethodDelete, "/api/v1/settings/ai-assistant", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestAIAssistantSettingsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []routeCase{
		{http.MethodGet, "/api/v1/settings/ai-assistant"},
		{http.MethodPut, "/api/v1/settings/ai-assistant"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHandleUpdateAIAssistantSettings_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db, _ := providerRouter(t)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_ai", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"active":{"provider":"anthropic","model":"claude-sonnet-5"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/ai-assistant", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the ai-assistant settings write", rec.Code, http.StatusForbidden)
	}
}
