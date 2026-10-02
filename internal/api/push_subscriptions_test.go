package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithPushVAPIDPublicKey wires a non-empty public key, the
// same "configured" state secretsManager != nil produces in
// cmd/levelrail/main.go, so the create/get-public-key routes don't 501.
func newTestRouterWithPushVAPIDPublicKey(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithPushVAPIDPublicKey("test-public-key")), db
}

func TestHandleGetPushVAPIDPublicKey_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/push-subscriptions/vapid-public-key", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleGetPushVAPIDPublicKey_Success(t *testing.T) {
	rt, db := newTestRouterWithPushVAPIDPublicKey(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/push-subscriptions/vapid-public-key", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["public_key"] != "test-public-key" {
		t.Errorf("public_key = %q, want %q", got["public_key"], "test-public-key")
	}
}

func TestHandleCreatePushSubscription_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"endpoint":"https://push.example.com/abc","keys":{"p256dh":"p","auth":"a"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/push-subscriptions", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleCreatePushSubscription_ThenList(t *testing.T) {
	rt, db := newTestRouterWithPushVAPIDPublicKey(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"endpoint":"https://push.example.com/abc","keys":{"p256dh":"p","auth":"a"},"user_agent":"Mozilla/5.0"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/push-subscriptions", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created pushSubscriptionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.UserAgent != "Mozilla/5.0" {
		t.Errorf("UserAgent = %q, want %q", created.UserAgent, "Mozilla/5.0")
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/push-subscriptions", ""))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var got []pushSubscriptionResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(got) != 1 || got[0].ID != created.ID {
		t.Errorf("list = %+v, want exactly the subscription just created", got)
	}
}

func TestHandleCreatePushSubscription_MissingFields_Rejected(t *testing.T) {
	rt, db := newTestRouterWithPushVAPIDPublicKey(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/push-subscriptions", `{"endpoint":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeletePushSubscription_Success(t *testing.T) {
	rt, db := newTestRouterWithPushVAPIDPublicKey(t)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/push-subscriptions",
		`{"endpoint":"https://push.example.com/abc","keys":{"p256dh":"p","auth":"a"}}`))
	var created pushSubscriptionResource
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	delRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(delRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/push-subscriptions/"+created.ID, ""))
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body = %s", delRec.Code, http.StatusNoContent, delRec.Body.String())
	}
}

func TestHandleDeletePushSubscription_NotFound(t *testing.T) {
	rt, db := newTestRouterWithPushVAPIDPublicKey(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/push-subscriptions/nope", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestPushSubscriptionRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouterWithPushVAPIDPublicKey(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/settings/push-subscriptions/vapid-public-key"},
		{http.MethodGet, "/api/v1/settings/push-subscriptions"},
		{http.MethodPost, "/api/v1/settings/push-subscriptions"},
		{http.MethodDelete, "/api/v1/settings/push-subscriptions/whatever"},
	})
}
