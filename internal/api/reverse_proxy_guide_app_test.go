package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestReverseProxyGuideApp(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.doctorHTTPPort = 8088
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 3000, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "bare", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		query    string
		want     int
		wantSnip string
	}{
		{"first domain of the app", "?app=web&proxy=nginx", http.StatusOK, "proxy_pass http://127.0.0.1:8088"},
		{"explicit domain", "?app=web&domain=app.example.com&proxy=traefik", http.StatusOK, "-web-http:"},
		{"foreign domain", "?app=web&domain=other.example.com", http.StatusBadRequest, ""},
		{"no domains", "?app=bare", http.StatusBadRequest, ""},
		{"unknown app", "?app=nope", http.StatusNotFound, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/reverse-proxy"+tc.query, ""))
			if rec.Code != tc.want {
				t.Fatalf("code = %d %s", rec.Code, rec.Body)
			}
			if tc.wantSnip == "" {
				return
			}
			var out reverseProxyGuideResource
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Plan == nil {
				t.Fatalf("plan missing: %s", rec.Body)
			}
			if out.Plan.Domain != "app.example.com" || !strings.Contains(out.Plan.Snippet, tc.wantSnip) {
				t.Errorf("plan = %+v", out.Plan)
			}
		})
	}
}
