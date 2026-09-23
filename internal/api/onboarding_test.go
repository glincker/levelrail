package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnboardingRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/onboarding", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/complete", nil),
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want %d", req.Method, req.URL.Path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestHandleGetOnboardingState_DefaultsIncomplete(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/onboarding", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got onboardingStateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Completed {
		t.Error("Completed = true, want false on a freshly migrated database")
	}
}

func TestHandleCompleteOnboarding(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/onboarding/complete", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got onboardingStateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Completed {
		t.Error("Completed = false, want true after POST /onboarding/complete")
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/onboarding", ""))
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Completed {
		t.Error("Completed = false on re-read, want true (persisted)")
	}
}

func TestHandleUpdateOnboardingProgress(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"current_step":"app","steps":{"server":"completed","domain":"skipped","git":"skipped"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/onboarding/progress", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/onboarding", ""))
	var got onboardingStateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CurrentStep != "app" {
		t.Errorf("CurrentStep = %q, want app", got.CurrentStep)
	}
	want := map[string]string{"server": "completed", "domain": "skipped", "git": "skipped"}
	if len(got.Steps) != len(want) {
		t.Fatalf("Steps = %v, want %v", got.Steps, want)
	}
	for k, v := range want {
		if got.Steps[k] != v {
			t.Errorf("Steps[%s] = %q, want %q", k, got.Steps[k], v)
		}
	}
	if got.Completed {
		t.Error("Completed = true, want false: saving progress must not complete onboarding")
	}
}

func TestHandleUpdateOnboardingProgress_Validation(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"malformed json", `{`},
		{"unknown current step", `{"current_step":"billing","steps":{}}`},
		{"unknown step id", `{"current_step":"server","steps":{"billing":"completed"}}`},
		{"unknown status", `{"current_step":"server","steps":{"server":"done"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/onboarding/progress", tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleUpdateOnboardingProgress_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/onboarding/progress", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleCompleteOnboarding_KeepsProgress(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/onboarding/progress", `{"current_step":"done","steps":{"app":"completed"}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("progress status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/onboarding/complete", ""))
	var got onboardingStateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Completed || got.CurrentStep != "done" || got.Steps["app"] != "completed" {
		t.Errorf("state = %+v, want completed with progress preserved", got)
	}
}

func TestHandleUpdateOnboardingProgress_ForbiddenForNonRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	u := storeUserWithAbilitiesForTest(t, db, "writer@example.com", []string{AbilityRead, AbilityWrite})
	cookie := sessionCookieForTest(t, rt, u.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/onboarding/progress", `{"current_step":"server","steps":{}}`))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
