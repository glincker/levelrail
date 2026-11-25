package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseIntegrations_ListAndDeny(t *testing.T) {
	t.Parallel()
	rt, db, _ := newBranchRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/databases/main/integrations", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/integrations", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Database     string `json:"database"`
		Integrations []struct {
			ID    string `json:"id"`
			State struct {
				Code string `json:"code"`
			} `json:"state"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Database != "main" || len(res.Integrations) == 0 {
		t.Fatalf("body = %s err = %v", rec.Body.String(), err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/integrations", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing database status = %d", rec.Code)
	}
}
