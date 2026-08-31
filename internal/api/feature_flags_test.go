package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedFeatureFlagApp(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app %q: %v", name, err)
	}
}

const featureFlagCreateBody = `{"key":"new-checkout","name":"New checkout","enabled":true,"rollout_percentage":100}`

func TestHandleCreateFeatureFlag_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", featureFlagCreateBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got featureFlagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" || got.Key != "new-checkout" || got.ServiceName != "web" || !got.Enabled || got.RolloutPercentage != 100 {
		t.Errorf("created resource = %+v, unexpected", got)
	}
}

func TestHandleCreateFeatureFlag_UnknownApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nonexistent/flags", featureFlagCreateBody))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleCreateFeatureFlag_EmptyKey_BadRequest(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"key":"","name":"New checkout","enabled":true,"rollout_percentage":100}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateFeatureFlag_InvalidKeyCharset_BadRequest(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"key":"New Checkout!","name":"New checkout","enabled":true,"rollout_percentage":100}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateFeatureFlag_RolloutOutOfRange_BadRequest(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"key":"new-checkout","name":"New checkout","enabled":true,"rollout_percentage":101}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateFeatureFlag_DuplicateKey_Conflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")
	seedFeatureFlagApp(t, db, "worker")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", featureFlagCreateBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/worker/flags", featureFlagCreateBody))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d (key is globally unique), body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleListFeatureFlags_ScopedToApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")
	seedFeatureFlagApp(t, db, "worker")

	create := func(app, key string) {
		rec := httptest.NewRecorder()
		body := `{"key":"` + key + `","name":"flag","enabled":true,"rollout_percentage":100}`
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+app+"/flags", body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create for %q status = %d, body = %s", app, rec.Code, rec.Body.String())
		}
	}
	create("web", "flag-a")
	create("web", "flag-b")
	create("worker", "flag-c")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/flags", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []featureFlagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("list = %d flags, want 2 (scoped to web only)", len(got))
	}
}

func TestHandleGetFeatureFlag_OtherAppOwned_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")
	seedFeatureFlagApp(t, db, "worker")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", featureFlagCreateBody))
	var created featureFlagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/worker/flags/"+created.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d: a flag belonging to a different app must not be reachable through it", rec.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateFeatureFlag_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", featureFlagCreateBody))
	var created featureFlagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = httptest.NewRecorder()
	updateBody := `{"key":"ignored-on-update","name":"New checkout v2","description":"desc","enabled":false,"rollout_percentage":25}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/flags/"+created.ID, updateBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got featureFlagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "New checkout v2" || got.Enabled || got.RolloutPercentage != 25 {
		t.Errorf("updated resource = %+v, unexpected", got)
	}
	if got.Key != "new-checkout" {
		t.Errorf("Key = %q, want unchanged %q (update must never rewrite the key)", got.Key, "new-checkout")
	}
}

func TestHandleUpdateFeatureFlag_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	rec := httptest.NewRecorder()
	updateBody := `{"name":"x","enabled":true,"rollout_percentage":50}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/flags/ff_missing", updateBody))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleDeleteFeatureFlag_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", featureFlagCreateBody))
	var created featureFlagResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/flags/"+created.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/flags/"+created.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status after delete = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// seedFlagWithReadToken creates a flag through the session-authed CRUD
// route, then mints a plain AbilityRead token: the minimally-scoped
// token an operator would actually inject into a running app as a
// secret env var, per this feature's own integration model.
func seedFlagWithReadToken(t *testing.T, rt *Router, db *store.DB, cookie *http.Cookie, key, body string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/flags", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create flag %q status = %d, body = %s", key, rec.Code, rec.Body.String())
	}

	const plaintext = "read-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(t.Context(), store.APIToken{
		ID: "tok_read_" + key, Name: "app runtime token", TokenHash: hashToken(plaintext + key), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed read token: %v", err)
	}
	return plaintext + key
}

func evaluateWithToken(t *testing.T, rt *Router, key, token, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flags/evaluate/"+key+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHandleEvaluateFeatureFlag_ReadScopedToken_FullRollout(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	token := seedFlagWithReadToken(t, rt, db, cookie, "new-checkout",
		`{"key":"new-checkout","name":"New checkout","enabled":true,"rollout_percentage":100}`)

	rec := evaluateWithToken(t, rt, "new-checkout", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s (a plain AbilityRead token must reach this route)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got evaluateFlagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Key != "new-checkout" || !got.Enabled {
		t.Errorf("evaluate response = %+v, want enabled=true at 100%% rollout", got)
	}
}

func TestHandleEvaluateFeatureFlag_DisabledFlag_AlwaysFalse(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	token := seedFlagWithReadToken(t, rt, db, cookie, "off-flag",
		`{"key":"off-flag","name":"Off flag","enabled":false,"rollout_percentage":100}`)

	rec := evaluateWithToken(t, rt, "off-flag", token, "")
	var got evaluateFlagResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Errorf("evaluate response = %+v, want enabled=false: Enabled=false is a hard kill switch regardless of rollout_percentage", got)
	}
}

func TestHandleEvaluateFeatureFlag_ZeroPercent_AlwaysFalse(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	token := seedFlagWithReadToken(t, rt, db, cookie, "zero-flag",
		`{"key":"zero-flag","name":"Zero flag","enabled":true,"rollout_percentage":0}`)

	for _, identifier := range []string{"", "user-1", "user-2", "user-3"} {
		rec := evaluateWithToken(t, rt, "zero-flag", token, "?identifier="+identifier)
		var got evaluateFlagResource
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Enabled {
			t.Errorf("identifier %q: evaluate response = %+v, want enabled=false at 0%% rollout", identifier, got)
		}
	}
}

func TestHandleEvaluateFeatureFlag_HundredPercent_AlwaysTrue(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	token := seedFlagWithReadToken(t, rt, db, cookie, "full-flag",
		`{"key":"full-flag","name":"Full flag","enabled":true,"rollout_percentage":100}`)

	for _, identifier := range []string{"", "user-1", "user-2", "user-3"} {
		rec := evaluateWithToken(t, rt, "full-flag", token, "?identifier="+identifier)
		var got evaluateFlagResource
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !got.Enabled {
			t.Errorf("identifier %q: evaluate response = %+v, want enabled=true at 100%% rollout", identifier, got)
		}
	}
}

// TestHandleEvaluateFeatureFlag_ConsistentPerIdentifier is the core
// determinism guarantee this feature's own spec requires: the same
// (flag, identifier) pair must land on the same side of a partial
// rollout on every call, never a coin flip that changes per request.
func TestHandleEvaluateFeatureFlag_ConsistentPerIdentifier(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")

	token := seedFlagWithReadToken(t, rt, db, cookie, "partial-flag",
		`{"key":"partial-flag","name":"Partial flag","enabled":true,"rollout_percentage":50}`)

	for _, identifier := range []string{"user-1", "user-2", "user-3", "", "device-abc"} {
		var first *bool
		for i := 0; i < 5; i++ {
			rec := evaluateWithToken(t, rt, "partial-flag", token, "?identifier="+identifier)
			var got evaluateFlagResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if first == nil {
				first = &got.Enabled
				continue
			}
			if got.Enabled != *first {
				t.Errorf("identifier %q: evaluation flipped between repeated calls (got %v, first was %v): rollout must be deterministic per identifier", identifier, got.Enabled, *first)
			}
		}
	}
}

func TestHandleEvaluateFeatureFlag_UnknownKey_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFeatureFlagApp(t, db, "web")
	token := seedFlagWithReadToken(t, rt, db, cookie, "some-flag",
		`{"key":"some-flag","name":"Some flag","enabled":true,"rollout_percentage":100}`)

	rec := evaluateWithToken(t, rt, "does-not-exist", token, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestEvaluateFeatureFlag_RolloutDistributionRoughlyMatchesPercentage(t *testing.T) {
	flag := store.FeatureFlag{ID: "ff_dist", Enabled: true, RolloutPercentage: 30}
	on := 0
	const n = 2000
	for i := 0; i < n; i++ {
		identifier := "user-" + string(rune('a'+i%26)) + string(rune('0'+i%10)) + string(rune('A'+i%26)) + string(rune(i))
		if evaluateFeatureFlag(flag, identifier) {
			on++
		}
	}
	pct := float64(on) / float64(n) * 100
	if pct < 20 || pct > 40 {
		t.Errorf("rollout at 30%% produced %.1f%% enabled across %d distinct identifiers, want roughly 30%% (20-40%% tolerance)", pct, n)
	}
}
