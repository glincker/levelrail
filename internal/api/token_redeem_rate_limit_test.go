package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postTokenRedeem(rt *Router, path, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestTokenRedeemRateLimit(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{"reset password", "/api/v1/auth/reset-password", `{"token":"nope","new_password":"long-enough-pw"}`},
		{"accept invite", "/api/v1/invites/accept", `{"token":"nope","password":"long-enough-pw"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			rt := NewRouter(discardLogger(), testBrand(), db, WithTokenRedeemRateLimit(2))

			for i := 0; i < 2; i++ {
				rec := postTokenRedeem(rt, tc.path, tc.body, "203.0.113.7:1000")
				if rec.Code == http.StatusTooManyRequests {
					t.Fatalf("attempt %d limited early: %s", i, rec.Body.String())
				}
			}
			rec := postTokenRedeem(rt, tc.path, tc.body, "203.0.113.7:1000")
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("third attempt status = %d, want 429", rec.Code)
			}
			if rec.Header().Get("Retry-After") == "" {
				t.Error("missing Retry-After")
			}
			if other := postTokenRedeem(rt, tc.path, tc.body, "198.51.100.9:1000"); other.Code == http.StatusTooManyRequests {
				t.Error("a different IP must have its own budget")
			}
		})
	}
}

func TestTokenRedeemRateLimit_DisabledByDefault(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	for i := 0; i < 20; i++ {
		rec := postTokenRedeem(rt, "/api/v1/auth/reset-password", `{"token":"nope","new_password":"long-enough-pw"}`, "203.0.113.7:1000")
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d limited without the option", i)
		}
	}
}
