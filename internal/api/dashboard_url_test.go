package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeDashboardURL(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: ""},
		{in: "  ", want: ""},
		{in: "https://dash.example.com", want: "https://dash.example.com"},
		{in: "https://dash.example.com/", want: "https://dash.example.com"},
		{in: " http://203.0.113.7:8080 ", want: "http://203.0.113.7:8080"},
		{in: "dash.example.com", wantErr: true},
		{in: "ftp://dash.example.com", wantErr: true},
		{in: "https://", wantErr: true},
		{in: "https://dash.example.com/app", wantErr: true},
		{in: "https://dash.example.com/?x=1", wantErr: true},
		{in: "https://user:pw@dash.example.com", wantErr: true}, //nolint:gosec // not a credential, a rejected-input case
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := normalizeDashboardURL(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDashboardURLHandlers(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/dashboard-url", `{"dashboard_url":"https://dash.example.com/"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("PUT https url over plain http: status = %d, want 409 (would lock the operator out)", rec.Code)
	}

	viaCaddy := authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/dashboard-url", `{"dashboard_url":"https://dash.example.com/"}`)
	viaCaddy.RemoteAddr = "127.0.0.1:40000"
	viaCaddy.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, viaCaddy)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/dashboard-url", ""))
	var got dashboardURLResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DashboardURL != "https://dash.example.com" {
		t.Errorf("GET dashboard_url = %q, want normalized https://dash.example.com", got.DashboardURL)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/dashboard-url", `{"dashboard_url":"not a url"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid PUT status = %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/settings/dashboard-url", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated PUT status = %d, want 401", rec.Code)
	}
}
