package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestIngressDNS01_StoredWinsOverEnvAndNeedsProvider(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	nudge := &fakeNudge{}
	rt.reconcileNudger = nudge
	const path = "/api/v1/settings/ingress/dns01"
	read := func(rec interface{ Bytes() []byte }) dns01Resource {
		var r dns01Resource
		if err := json.Unmarshal(rec.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Setenv("APP_INGRESS_DNS01_ALL_HOSTS", "true")
	rec := rsDo(t, rt, cookie, http.MethodGet, path, "")
	if got := read(rec.Body); rec.Code != http.StatusOK || !got.Enabled || got.Source != "default" {
		t.Fatalf("env seed = %d %+v", rec.Code, got)
	}

	if rec = rsDo(t, rt, cookie, http.MethodPut, path, `{"enabled":true}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "DNS provider") {
		t.Fatalf("enable without provider = %d %s", rec.Code, rec.Body.String())
	}

	// Disabling needs no provider, and the stored value beats the env seed at once.
	if rec = rsDo(t, rt, cookie, http.MethodPut, path, `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d %s", rec.Code, rec.Body.String())
	}
	if got := read(rec.Body); got.Enabled || got.Source != "stored" || nudge.count != 1 {
		t.Fatalf("after disable = %+v nudges %d", got, nudge.count)
	}

	t.Setenv("APP_INGRESS_DNS01_ALL_HOSTS", "false")
	rt.SetDNS01ProviderCheck(func(context.Context) bool { return true })
	if rec = rsDo(t, rt, cookie, http.MethodPut, path, `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}
	if got := read(rec.Body); !got.Enabled || got.Source != "stored" || !got.ProviderConnected || nudge.count != 2 {
		t.Fatalf("after enable = %+v nudges %d", got, nudge.count)
	}
}
