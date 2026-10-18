package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestNormalizeFunctionPath(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"hello", "/hello", false},
		{"/hello/", "/hello", false},
		{" /a/b ", "/a/b", false},
		{"/", "", true},
		{"", "", true},
	}
	for _, tc := range tests {
		got, err := normalizeFunctionPath(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("normalizeFunctionPath(%q) = %q, %v; want %q err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestUpsertFunctionRuleOrder(t *testing.T) {
	mk := func(name, path string) trafficpolicy.Forwarder {
		return trafficpolicy.Forwarder{Name: name, Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: path}}
	}
	rules := []trafficpolicy.Forwarder{mk("", "/plain"), mk("function:a", "/a")}
	got := upsertFunctionRule(rules, mk("function:b", "/a/deep"))
	want := []string{"/a/deep", "/a", "/plain"}
	for i, w := range want {
		if got[i].Match.Path != w {
			t.Fatalf("order = %+v, want %v", got, want)
		}
	}
	again := upsertFunctionRule(got, mk("function:b", "/a/deep"))
	if len(again) != len(got) {
		t.Fatalf("same function and path must replace, got %d rules", len(again))
	}
}

func TestFunctionRouteLifecycle(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, s := range []store.DesiredService{
		{Name: "site", Image: "nginx:1", Port: 80, Domains: []string{"example.com"}},
		{Name: "fn", Image: "nginx:1", Port: 80},
	} {
		if err := db.SaveDesiredService(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	if rec := do(http.MethodPut, "/api/v1/functions/fn/route", `{"domain":"nope.example.org","path":"/hi"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unowned domain = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, "/api/v1/functions/fn/route", `{"domain":"example.com","path":"/hi"}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d: %s", rec.Code, rec.Body.String())
	}
	rec := do(http.MethodGet, "/api/v1/functions/fn/route", "")
	var got functionRoutesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Routes) != 1 || got.Routes[0] != (FunctionRoute{Domain: "example.com", Path: "/hi", StripPrefix: true}) {
		t.Fatalf("routes = %+v", got.Routes)
	}
	rec = do(http.MethodDelete, "/api/v1/functions/fn/route?domain=example.com", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
	got = functionRoutesResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Routes) != 0 {
		t.Fatalf("after delete routes = %+v err=%v", got.Routes, err)
	}
}
