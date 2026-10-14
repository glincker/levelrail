package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/eventhook"
)

func TestEventWebhooks_CRUDSecretShownOnce(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetEventWebhooks(eventhook.NewService(eventhook.Deps{
		Store: db, Secrets: &exportTestSecrets{m: map[string]string{}}, Brand: "plat",
	}))
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := do(http.MethodPost, "/api/v1/event-webhooks", `{"name":"siem","url":"https://203.0.113.9/hook","events":["apps.*"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created eventWebhookResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !strings.HasPrefix(created.SigningSecret, "whsec_") || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}

	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"private url", `{"name":"x","url":"http://127.0.0.1:9000"}`, http.StatusBadRequest},
		{"empty name", `{"name":"","url":"https://203.0.113.9"}`, http.StatusBadRequest},
		{"duplicate name", `{"name":"siem","url":"https://203.0.113.9"}`, http.StatusConflict},
	} {
		if rec := do(http.MethodPost, "/api/v1/event-webhooks", tc.body); rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d, body = %s", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}

	rec = do(http.MethodGet, "/api/v1/event-webhooks/"+created.ID, "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "whsec_") || strings.Contains(rec.Body.String(), "signing_secret") {
		t.Fatalf("get leaked a secret or failed: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodPatch, "/api/v1/event-webhooks/"+created.ID, `{"enabled":false}`)
	var patched eventWebhookResource
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if rec.Code != http.StatusOK || patched.Enabled || len(patched.Events) != 1 {
		t.Fatalf("patched = %d %+v", rec.Code, patched)
	}

	rec = do(http.MethodPost, "/api/v1/event-webhooks/"+created.ID+"/rotate-secret", "")
	var rotated eventWebhookResource
	_ = json.Unmarshal(rec.Body.Bytes(), &rotated)
	if rec.Code != http.StatusOK || rotated.SigningSecret == "" || rotated.SigningSecret == created.SigningSecret {
		t.Fatalf("rotate = %d %s", rec.Code, rec.Body.String())
	}

	if rec := do(http.MethodGet, "/api/v1/event-webhooks/"+created.ID+"/deliveries", ""); rec.Code != http.StatusOK {
		t.Fatalf("deliveries status = %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/event-webhook-deliveries/dlv_missing/replay", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("replay missing status = %d", rec.Code)
	}
	if rec := do(http.MethodDelete, "/api/v1/event-webhooks/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/api/v1/event-webhooks/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d", rec.Code)
	}
}

func TestEventWebhooks_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/event-webhooks", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}
