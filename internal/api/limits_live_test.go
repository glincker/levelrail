package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func putLimit(t *testing.T, rt *Router, cookie *http.Cookie, key, value string) int {
	t.Helper()
	return rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+key, `{"value":`+value+`,"confirm_weakening":true}`).Code
}

func TestLimitSettings_ReachConsumersWithoutRestart(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.apiRateLimit = newAPIRateLimit(600, 120)
	rt.attachLiveLimits()
	// Raising asks for confirmation; the first writes below lower the budget, which does not.

	if code := putLimit(t, rt, cookie, runtimesettings.KeyAPIRateLimitWrite, "20"); code != http.StatusOK {
		t.Fatalf("put write rpm = %d", code)
	}
	for i := 0; i < 20; i++ {
		if ok, _ := rt.apiRateLimit.writes.allow("k"); !ok {
			t.Fatalf("request %d refused under the new budget of 20", i+1)
		}
	}
	if ok, _ := rt.apiRateLimit.writes.allow("k"); ok {
		t.Fatal("request 21 passed although the saved budget is 20")
	}
	// A larger budget must not forgive the caller that already drained its bucket.
	if code := putLimit(t, rt, cookie, runtimesettings.KeyAPIRateLimitWrite, "5000"); code != http.StatusOK {
		t.Fatalf("raise write rpm = %d", code)
	}
	if ok, _ := rt.apiRateLimit.writes.allow("k"); ok {
		t.Fatal("raising the limit reset a drained bucket")
	}

	if code := putLimit(t, rt, cookie, runtimesettings.KeyAPIMaxBodyBytes, "1048576"); code != http.StatusOK {
		t.Fatalf("put body limit = %d", code)
	}
	h := rt.bodyLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(strings.Repeat("a", 2<<20))))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("2 MiB body under a saved 1 MiB limit = %d", rec.Code)
	}

	if code := putLimit(t, rt, cookie, runtimesettings.KeyDeployMaxConcurrent, "2"); code != http.StatusOK {
		t.Fatalf("put deploy concurrency = %d", code)
	}
	if got := rt.deployCap(t.Context()); got != 2 {
		t.Fatalf("deploy cap = %d, want 2", got)
	}
}

func TestLimitSettings_RejectInvalidValues(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	cases := []struct{ key, value string }{
		{runtimesettings.KeyAPIRateLimitRead, "0"},
		{runtimesettings.KeyAPIRateLimitRead, "-1"},
		{runtimesettings.KeyAPIRateLimitWrite, "5"},
		{runtimesettings.KeyAPIRateLimitRedeem, "0"},
		{runtimesettings.KeyAuthRateLimitPerIP, "0"},
		{runtimesettings.KeyAuthRateLimitPerEmail, "-3"},
		{runtimesettings.KeyAPIMaxBodyBytes, "0"},
		{runtimesettings.KeyAPIMaxBodyBytes, "4294967296"},
		{runtimesettings.KeyAssetsUploadMaxBytes, "1024"},
		{runtimesettings.KeyDeployMaxConcurrent, "-1"},
		{runtimesettings.KeyDeployMaxConcurrent, `"many"`},
	}
	for _, c := range cases {
		if code := putLimit(t, rt, cookie, c.key, c.value); code != http.StatusBadRequest {
			t.Errorf("PUT %s=%s = %d, want 400", c.key, c.value, code)
		}
	}
}

func TestLimitSettings_AuthFloorAndRestart(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	below := runtimesettings.AuthRateLimitFloor - 1
	for _, key := range []string{runtimesettings.KeyAuthRateLimitPerIP, runtimesettings.KeyAuthRateLimitPerEmail} {
		if code := putLimit(t, rt, cookie, key, strconv.Itoa(below)); code != http.StatusBadRequest {
			t.Errorf("%s below the floor = %d, want 400", key, code)
		}
		if code := putLimit(t, rt, cookie, key, strconv.Itoa(below+1)); code != http.StatusOK {
			t.Errorf("%s at the floor = %d, want 200", key, code)
		}
	}
	rec := rsDo(t, rt, cookie, http.MethodGet, rsBase, "")
	if !strings.Contains(rec.Body.String(), `"pending_restart":true`) {
		t.Fatalf("auth limit change should wait for a restart: %s", rec.Body.String())
	}
}

func TestLimitSettings_EnvSeedsAreClampedOnTheLiveLimiter(t *testing.T) {
	t.Setenv("APP_API_RATE_LIMIT_READ_RPM", "0")
	t.Setenv("APP_API_RATE_LIMIT_WRITE_RPM", "5")
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithAPIRateLimit(0, 5))
	if got := rt.apiRateLimit.reads.refresh(); got != 600 {
		t.Fatalf("read budget with env 0 = %v, want the default 600 (a limit cannot be switched off)", got)
	}
	if got := rt.apiRateLimit.writes.refresh(); got != 20 {
		t.Fatalf("write budget with env 5 = %v, want the floor 20", got)
	}
	ctx := t.Context()
	if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyAPIRateLimitWrite, []byte("50"), "root"); err != nil {
		t.Fatal(err)
	}
	if got := rt.apiRateLimit.writes.refresh(); got != 50 {
		t.Fatalf("saved write budget = %v, want 50", got)
	}
	if _, err := rt.runtimeSettings.Reset(ctx, runtimesettings.KeyAPIRateLimitWrite); err != nil {
		t.Fatal(err)
	}
	if got := rt.apiRateLimit.writes.refresh(); got != 20 {
		t.Fatalf("write budget after reset = %v, want the clamped env value 20", got)
	}
}

func TestLimitSettings_RaiseNeedsConfirmation(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	key := runtimesettings.KeyAPIRateLimitRead
	rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+key, `{"value":5000}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "confirmation_required") || !strings.Contains(rec.Body.String(), "limit raised") {
		t.Fatalf("raise without confirmation = %d %s", rec.Code, rec.Body.String())
	}
	if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+key, `{"value":5000,"confirm_weakening":true}`); rec.Code != http.StatusOK {
		t.Fatalf("raise with confirmation = %d %s", rec.Code, rec.Body.String())
	}
	if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+key, `{"value":100}`); rec.Code != http.StatusOK {
		t.Fatalf("lowering = %d %s", rec.Code, rec.Body.String())
	}
}

func TestBodyLimit_DirectAssetUploadIsExempt(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if code := putLimit(t, rt, cookie, runtimesettings.KeyAPIMaxBodyBytes, "1048576"); code != http.StatusOK {
		t.Fatalf("lowering the body limit = %d", code)
	}
	h := rt.bodyLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	for path, want := range map[string]int{
		"/api/v1/assets/buckets/b1/objects": http.StatusOK,
		"/api/v1/apps/x/env":                http.StatusRequestEntityTooLarge,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, path, strings.NewReader(strings.Repeat("a", 20<<20))))
		if rec.Code != want {
			t.Errorf("PUT %s of 20 MiB = %d, want %d", path, rec.Code, want)
		}
	}
}
