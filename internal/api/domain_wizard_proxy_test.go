package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCheckDomain_UpstreamProxyOwnsCertificates(t *testing.T) {
	tests := []struct {
		name     string
		upstream bool
		want     string
	}{
		{"own certificates", false, challengeHTTP01},
		{"behind a proxy", true, challengeUpstreamProxy},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(context.Context, string) ([]string, error) {
				return []string{"203.0.113.10"}, nil
			})
			seedAppWithDomains(t, db, "app.example.com")
			s, err := db.GetIngressSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			s.TLSTerminatedUpstream = tc.upstream
			if err := db.UpdateIngressSettings(context.Background(), s); err != nil {
				t.Fatal(err)
			}
			cookie := loginTestSession(t, rt, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/check", ""))
			var got domainCheckResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Challenge != tc.want {
				t.Fatalf("challenge = %q, want %q", got.Challenge, tc.want)
			}
		})
	}
}
