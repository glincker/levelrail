package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func benchAuthedRequest(cookie *http.Cookie, method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.AddCookie(cookie)
	return req
}

func benchBearer(method, target, token string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func mustDecodeBench(b testing.TB, rec *httptest.ResponseRecorder, want int, dst any) {
	b.Helper()
	if rec.Code != want {
		b.Fatalf("status = %d, want %d, body = %s", rec.Code, want, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		b.Fatalf("decode: %v", err)
	}
}

func benchSeedApps(b testing.TB, db *store.DB, n int) {
	b.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		svc := store.DesiredService{Name: fmt.Sprintf("app-%05d", i), Image: "nginx:1", Port: 80, Labels: map[string]string{"tier": "web"}}
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			b.Fatal(err)
		}
	}
}

// benchScopedToken mints a read token with a deny policy so the caller takes the filtered list path.
func benchScopedToken(b testing.TB, rt *Router, cookie *http.Cookie) string {
	b.Helper()
	do := func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, benchAuthedRequest(cookie, method, target, body))
		return rec
	}
	var tok createTokenResponse
	mustDecodeBench(b, do(http.MethodPost, "/api/v1/auth/tokens", `{"name":"bench","abilities":["read"]}`), http.StatusCreated, &tok)
	doc := `{"Statement":[{"Effect":"Deny","Action":["read"],"Resource":["app:app-00000"]}]}`
	var pol policyResource
	mustDecodeBench(b, do(http.MethodPost, "/api/v1/iam/policies", `{"name":"bench-hide","document":`+doc+`}`), http.StatusCreated, &pol)
	rec := do(http.MethodPost, "/api/v1/iam/policies/"+pol.ID+"/attachments", `{"principal_type":"token","principal_id":"`+tok.ID+`"}`)
	if rec.Code != http.StatusNoContent {
		b.Fatalf("attach policy: %d %s", rec.Code, rec.Body.String())
	}
	return tok.Token
}

// BenchmarkListApps compares the plain path with the scoped caller path that loads every app and pages in memory.
func BenchmarkListApps(b *testing.B) {
	for _, n := range []int{100, 500, 2000, 10000} {
		rt, db := newTestRouter(b)
		cookie, err := loginTestSessionSlow(rt, db)
		if err != nil {
			b.Fatal(err)
		}
		benchSeedApps(b, db, n)
		scoped := benchScopedToken(b, rt, cookie)
		h := rt.Handler()
		cases := []struct {
			name string
			req  func() *http.Request
		}{
			{"admin", func() *http.Request { return benchAuthedRequest(cookie, http.MethodGet, "/api/v1/apps?limit=50", "") }},
			{"scoped", func() *http.Request { return benchBearer(http.MethodGet, "/api/v1/apps?limit=50", scoped) }},
		}
		for _, c := range cases {
			b.Run(fmt.Sprintf("apps=%d/%s", n, c.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, c.req())
					if rec.Code != http.StatusOK {
						b.Fatalf("list apps: %d %s", rec.Code, rec.Body.String())
					}
				}
			})
		}
	}
}
