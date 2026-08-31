package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGitPushWebhook_RecordsDelivery(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	rt.builder = &fakeBuilder{tag: "web:sha1"}

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	deliveries, err := db.ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("len(deliveries) = %d, want 1", len(deliveries))
	}
	d := deliveries[0]
	if d.Provider != "github" || d.EventType != "push" || !d.SignatureValid || !d.Matched || d.StatusCode != http.StatusOK {
		t.Errorf("delivery = %+v, want provider=github event_type=push signature_valid=true matched=true status_code=200", d)
	}
	if string(d.Payload) != string(body) {
		t.Errorf("delivery payload = %q, want %q", d.Payload, body)
	}
}

func TestHandleGitPushWebhook_WrongSignature_RecordsDelivery(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("wrong-secret"), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	deliveries, err := db.ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].SignatureValid || deliveries[0].StatusCode != http.StatusUnauthorized {
		t.Errorf("deliveries = %+v, want one unverified 401 record", deliveries)
	}
}

func TestHandleGitPushWebhook_NoGitSource_RecordsUnmatchedDelivery(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	seedApp(t, db, "web")

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	deliveries, err := db.ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Matched || deliveries[0].StatusCode != http.StatusNotFound {
		t.Errorf("deliveries = %+v, want one unmatched 404 record", deliveries)
	}
}

func TestHandleListWebhookDeliveries_AppNotFound(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/webhook-deliveries", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleReplayWebhookDelivery_TriggersDeploy(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed delivery: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fb.calls != 1 {
		t.Fatalf("builder calls after original delivery = %d, want 1", fb.calls)
	}

	deliveries, err := db.ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("len(deliveries) = %d, want 1", len(deliveries))
	}
	deliveryID := deliveries[0].ID

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/webhook-deliveries/"+deliveryID+"/replay", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("replay: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fb.calls != 2 {
		t.Fatalf("builder calls after replay = %d, want 2", fb.calls)
	}

	var result replayWebhookDeliveryResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("result.Status = %d, want 200", result.Status)
	}

	// Replay does not create a second webhook_deliveries row, see
	// handleReplayWebhookDelivery's own doc comment for why.
	deliveries, err = db.ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("len(deliveries) after replay = %d, want 1", len(deliveries))
	}
}

func TestHandleReplayWebhookDelivery_RequiresAbilityDeploy(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	seedApp(t, db, "web")

	id, err := store.NewWebhookDeliveryID()
	if err != nil {
		t.Fatalf("NewWebhookDeliveryID() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/webhook-deliveries/"+id+"/replay", nil)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with no session/token", rec.Code)
	}
}

func TestHandleReplayWebhookDelivery_WrongApp_NotFound(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "other")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
	rec := httptest.NewRecorder()
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	rt.builder = &fakeBuilder{tag: "web:sha1"}
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed delivery: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	deliveries, err := db.ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	deliveryID := deliveries[0].ID

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/other/webhook-deliveries/"+deliveryID+"/replay", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: web's delivery id must not replay under app other", rec.Code)
	}
}
