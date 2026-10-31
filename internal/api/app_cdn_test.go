package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppCDN_OverrideRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")
	cookie := loginTestSession(t, rt, db)
	const path = "/api/v1/apps/web/cdn"

	get := func(method, body string) appCDNResource {
		t.Helper()
		rec := doPolicy(t, rt, cookie, method, path, body)
		var res appCDNResource
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
			t.Fatalf("%s = %d: %s", method, rec.Code, rec.Body.String())
		}
		return res
	}
	if res := get(http.MethodGet, ""); res.Proxied != nil || res.Effective || res.GlobalProxied {
		t.Fatalf("default = %+v", res)
	}
	if res := get(http.MethodPut, `{"proxied":true}`); res.Proxied == nil || !*res.Proxied || !res.Effective {
		t.Fatalf("after on = %+v", res)
	}
	if v, set, err := db.GetAppDNSProxied(context.Background(), "web"); err != nil || !set || !v {
		t.Fatalf("stored v=%v set=%v err=%v", v, set, err)
	}
	if res := get(http.MethodPut, `{"proxied":null}`); res.Proxied != nil || res.Effective {
		t.Fatalf("after clear = %+v", res)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, path, `{"bogus":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", rec.Code)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/nope/cdn", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", rec.Code)
	}
}

func TestAppCDN_AppOverridesGlobal(t *testing.T) {
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")
	off := false
	if err := db.SetAppDNSProxied(context.Background(), "web", &off, "t"); err != nil {
		t.Fatal(err)
	}
	if rt.appDNSProxied(context.Background(), "web", store.IngressSettings{DNSProxied: true}) {
		t.Fatal("an explicit off must beat the global on")
	}
	if !rt.appDNSProxied(context.Background(), "other", store.IngressSettings{DNSProxied: true}) {
		t.Fatal("an app without an override follows the global setting")
	}
}
