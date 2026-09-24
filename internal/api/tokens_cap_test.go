package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCreateToken_PrivilegeCap(t *testing.T) {
	tests := []struct {
		name       string
		caller     []string
		body       string
		wantStatus int
	}{
		{"read caller cannot mint root", []string{AbilityRead}, `{"name":"x","abilities":["root"]}`, http.StatusForbidden},
		{"read caller cannot mint deploy", []string{AbilityRead}, `{"name":"x","abilities":["read","deploy"]}`, http.StatusForbidden},
		{"read caller can mint read", []string{AbilityRead}, `{"name":"x","abilities":["read"]}`, http.StatusCreated},
		{"root caller can mint deploy", []string{AbilityRoot}, `{"name":"x","abilities":["deploy"]}`, http.StatusCreated},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			bootstrapTestAdmin(t, db)
			caller := storeUserWithAbilitiesForTest(t, db, fmt.Sprintf("tok-caller-%d@example.com", i), tt.caller)
			cookie := sessionCookieForTest(t, rt, caller.ID)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", tt.body))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
