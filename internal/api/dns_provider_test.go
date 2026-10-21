package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDNSProviderSetAndResolutionTest(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithPublicHost("203.0.113.5"))
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	if rec := do(http.MethodPut, "/api/v1/settings/dns-provider", `{"provider":"custom","custom_base_domain":"co.uk"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("public suffix accepted: %d", rec.Code)
	}
	if rec := do(http.MethodPut, "/api/v1/settings/dns-provider", `{"provider":"nip.io"}`); rec.Code != http.StatusOK {
		t.Fatalf("set default: %d %s", rec.Code, rec.Body.String())
	}
	if host, ok := rt.autoHost(t.Context(), "web"); !ok || host != "web.203-0-113-5.nip.io" {
		t.Fatalf("auto host = %q ok=%v", host, ok)
	}

	orig := dnsLookupHost
	t.Cleanup(func() { dnsLookupHost = orig })
	cases := []struct {
		name   string
		lookup func(context.Context, string) ([]string, error)
		want   string
	}{
		{"ok", func(context.Context, string) ([]string, error) { return []string{"203.0.113.5"}, nil }, dnsTestOK},
		{"wrong ip", func(context.Context, string) ([]string, error) { return []string{"198.51.100.9"}, nil }, dnsTestWrongIP},
		{"not resolving", func(context.Context, string) ([]string, error) { return nil, errors.New("nxdomain") }, dnsTestNotResolving},
	}
	for _, tc := range cases {
		dnsLookupHost = tc.lookup
		rec := do(http.MethodPost, "/api/v1/settings/dns-provider/test", `{}`)
		var res dnsProviderTestResult
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Status != tc.want {
			t.Fatalf("%s: status %q (%v), want %q", tc.name, res.Status, err, tc.want)
		}
		if tc.want != dnsTestOK && len(res.Fixes) == 0 {
			t.Errorf("%s: no fixes offered", tc.name)
		}
		if res.DigCommand == "" {
			t.Errorf("%s: no dig command", tc.name)
		}
	}
}
