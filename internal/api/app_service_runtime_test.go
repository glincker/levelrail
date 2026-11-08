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

func TestServiceRuntimeOptions(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	const path = "/api/v1/apps/web/services/web/runtime"
	put := func(rev, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := authedRequest(t, cookie, http.MethodPut, path, body)
		if rev != "" {
			req.Header.Set("If-Match", `"`+rev+`"`)
		}
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	get := func() serviceRuntimeResource {
		var res serviceRuntimeResource
		rec := budgetDo(t, rt, cookie, http.MethodGet, path, "")
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.Bounds.DNSMax == 0 {
			t.Fatalf("get = %d %s", rec.Code, rec.Body)
		}
		return res
	}
	if rec := budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/services/ghost/runtime", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown service = %d", rec.Code)
	}
	first := get()
	if rec := put("", `{"init":true}`); rec.Code != http.StatusPreconditionRequired {
		t.Errorf("no If-Match = %d", rec.Code)
	}
	if rec := put("stale", `{"init":true}`); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("stale If-Match = %d", rec.Code)
	}

	rec := put(first.Revision, `{"privileged":true,"cap_add":["SYS_ADMIN"],"dns":["not-an-ip"],"tmpfs":["/run"],"sysctls":{"kernel.shmmax":"1"},"stop_timeout":999999}`)
	var bad serviceRuntimeErrors
	if rec.Code != http.StatusUnprocessableEntity || json.Unmarshal(rec.Body.Bytes(), &bad) != nil {
		t.Fatalf("invalid put = %d %s", rec.Code, rec.Body)
	}
	for _, k := range []string{"privileged", "cap_add", "dns", "tmpfs", "sysctls", "stop_timeout"} {
		if bad.Errors[k] == "" {
			t.Errorf("no error for %s: %v", k, bad.Errors)
		}
	}
	if get().Revision != first.Revision {
		t.Error("a refused write must change nothing")
	}

	rec = put(first.Revision, `{"init":true,"dns":["1.1.1.1"],"tmpfs":["/run:size=16m"],"stop_timeout":30,"ulimits":{"nofile":{"soft":1024,"hard":4096}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	saved := get()
	if saved.Revision == first.Revision || saved.Runtime.Init == nil || !*saved.Runtime.Init || len(saved.Runtime.DNS) != 1 || saved.Runtime.Ulimits["nofile"].Hard != 4096 {
		t.Fatalf("not saved: %+v", saved)
	}
	if rec := put(first.Revision, `{"init":false}`); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("old revision after a write = %d", rec.Code)
	}
	if rec := put(saved.Revision, `{}`); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "1.1.1.1") {
		t.Errorf("empty body must clear: %d %s", rec.Code, rec.Body)
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil || svc.Runtime != nil {
		t.Errorf("runtime not cleared: %+v %v", svc.Runtime, err)
	}
}

func TestServiceRuntimeRefusesNonMember(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, n := range []string{"shop", "shop-billing"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "nginx:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/v1/apps/shop/services/billing/runtime"
	if rec := budgetDo(t, rt, cookie, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
		t.Errorf("get non-member = %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	req := authedRequest(t, cookie, http.MethodPut, path, `{"init":true}`)
	req.Header.Set("If-Match", `"x"`)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("put non-member = %d", rec.Code)
	}
	if svc, err := db.GetDesiredService(ctx, "shop-billing"); err != nil || svc.Runtime != nil {
		t.Errorf("other app changed: %+v %v", svc, err)
	}
}
