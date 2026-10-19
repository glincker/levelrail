package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestListFunctionsOnlyHoldRequestsApps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, name := range []string{"fn", "plain"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "nginx:1", Port: 80, Domains: []string{name + ".example.com"}}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetAppSleepIdle(ctx, name, 15, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetAppSleepHold(ctx, "fn", true, time.Now()); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/functions", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got functionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Functions) != 1 || got.Functions[0].Name != "fn" || got.Functions[0].IdleMinutes != 15 || got.Functions[0].URL != "https://fn.example.com" {
		t.Fatalf("functions = %+v", got.Functions)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/functions?window=3y", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad window = %d, want 400", rec.Code)
	}
}

func TestFunctionRuntimesRoute(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/functions/runtimes", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got struct {
		Runtimes []struct {
			ID string `json:"id"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Runtimes) != 3 {
		t.Fatalf("runtimes = %s (%v)", rec.Body.String(), err)
	}
}
