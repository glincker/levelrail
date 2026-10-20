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

func seedAppIntegrationApp(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app %q: %v", name, err)
	}
}

func TestHandleListIntegrationCatalog(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/integrations", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []integrationCatalogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("catalog response is empty, want at least one entry")
	}
	foundSentry := false
	for _, entry := range got {
		if entry.Key == "sentry" {
			foundSentry = true
			if len(entry.EnvVars) == 0 || entry.EnvVars[0].Name != "SENTRY_DSN" {
				t.Errorf("sentry entry env vars = %+v, want SENTRY_DSN first", entry.EnvVars)
			}
		}
	}
	if !foundSentry {
		t.Error("catalog response missing the sentry entry")
	}
}

func TestHandleAttachAppIntegration_NoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithSecretSetter
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"integration_key":"sentry","fields":{"SENTRY_DSN":"https://key@o0.ingest.sentry.io/0"}}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleAttachAppIntegration_Success(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"integration_key":"sentry","fields":{"SENTRY_DSN":"https://key@o0.ingest.sentry.io/0"}}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got appIntegrationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" || got.AppName != "web" || got.IntegrationKey != "sentry" || got.Name != "Sentry" {
		t.Errorf("attached resource = %+v, unexpected", got)
	}
	if setter.lastService != "app-integration/web/sentry" || setter.lastKey != "SENTRY_DSN" || setter.lastValue != "https://key@o0.ingest.sentry.io/0" {
		t.Errorf("SetValueGuarded call = (%q, %q, %q), unexpected", setter.lastService, setter.lastKey, setter.lastValue)
	}

	// The value is never echoed back.
	if strings.Contains(rec.Body.String(), "https://key@o0.ingest.sentry.io/0") {
		t.Error("response body must never echo the field value back")
	}
}

func TestHandleAttachAppIntegration_MissingRequiredField(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"integration_key":"sentry","fields":{}}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleAttachAppIntegration_UnknownKey(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"integration_key":"not-a-real-tool","fields":{}}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleAttachAppIntegration_AlreadyAttached(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")

	body := `{"integration_key":"sentry","fields":{"SENTRY_DSN":"https://key@o0.ingest.sentry.io/0"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first attach status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", body))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second attach status = %d, want %d, body = %s", rec2.Code, http.StatusConflict, rec2.Body.String())
	}
}

func TestHandleListAppIntegrations(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")

	attachBody := `{"integration_key":"sentry","fields":{"SENTRY_DSN":"https://key@o0.ingest.sentry.io/0"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", attachBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/integrations", ""))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var got []appIntegrationResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].IntegrationKey != "sentry" {
		t.Errorf("list = %+v, want one sentry attachment", got)
	}
}

func TestHandleDetachAppIntegration(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")

	attachBody := `{"integration_key":"sentry","fields":{"SENTRY_DSN":"https://key@o0.ingest.sentry.io/0"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", attachBody))
	var attached appIntegrationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &attached); err != nil {
		t.Fatalf("decode: %v", err)
	}

	delRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(delRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/integrations/"+attached.ID, ""))
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d, want %d, body = %s", delRec.Code, http.StatusNoContent, delRec.Body.String())
	}
	if len(setter.deletedNamespaces) != 1 || setter.deletedNamespaces[0] != "app-integration/web/sentry" {
		t.Errorf("deletedNamespaces = %v, want [app-integration/web/sentry]", setter.deletedNamespaces)
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/integrations", ""))
	var got []appIntegrationResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("list after detach = %+v, want empty", got)
	}
}

func TestHandleDetachAppIntegration_WrongApp(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedAppIntegrationApp(t, db, "web")
	seedAppIntegrationApp(t, db, "other")

	attachBody := `{"integration_key":"sentry","fields":{"SENTRY_DSN":"https://key@o0.ingest.sentry.io/0"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/integrations", attachBody))
	var attached appIntegrationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &attached); err != nil {
		t.Fatalf("decode: %v", err)
	}

	delRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(delRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/other/integrations/"+attached.ID, ""))
	if delRec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", delRec.Code, http.StatusNotFound, delRec.Body.String())
	}
}
