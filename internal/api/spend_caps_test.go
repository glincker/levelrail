package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/store"
)

func budgetDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestBudgetLifecycle(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}

	if rec := budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/budget", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("empty get = %d", rec.Code)
	}

	for name, body := range map[string]string{
		"no limit":    `{"action":"pause"}`,
		"bad action":  `{"monthly_egress_bytes":1000,"action":"explode"}`,
		"bad percent": `{"monthly_egress_bytes":1000,"thresholds":[0]}`,
		"junk":        `nope`,
	} {
		if rec := budgetDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/budget", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
	if rec := budgetDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/ghost/budget", `{"monthly_egress_bytes":1000}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown app = %d", rec.Code)
	}

	rec := budgetDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/budget", `{"monthly_egress_bytes":1000,"action":"throttle"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	var res spendCapResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Action != spendcap.ActionThrottle || res.ThrottleRPS != spendcap.DefaultThrottleRPS || len(res.Thresholds) != 3 || res.Condition.Reason != spendcap.ReasonWithinBudget {
		t.Errorf("status = %+v", res)
	}

	rec = budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/budget/history", "")
	var hist spendCapHistoryResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &hist) != nil || len(hist.Events) != 1 || hist.Events[0].Kind != spendcap.EventConfigured {
		t.Fatalf("history = %d %s", rec.Code, rec.Body)
	}
	if rec := budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/budget/history?limit=0", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad limit = %d", rec.Code)
	}

	if rec := budgetDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/budget", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := budgetDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/budget", ""); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d", rec.Code)
	}
	rec = budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/budget/history", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil || len(hist.Events) != 2 || hist.Events[0].Kind != spendcap.EventCleared {
		t.Errorf("history after clear = %s", rec.Body)
	}
}

func TestBudgetAppDeleteCascades(t *testing.T) {
	_, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	cp := spendcap.Cap{App: "web", MonthlyEgressBytes: 5, Action: spendcap.ActionPause}.Normalize()
	if err := db.SaveSpendCap(ctx, cp, spendcap.Event{App: "web", Kind: spendcap.EventConfigured}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSpendCap(ctx, "web"); err == nil {
		t.Error("cap survived its app")
	}
}
