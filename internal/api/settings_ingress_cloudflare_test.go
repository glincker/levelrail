package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestIngressCloudflarePresetNeedsConfirmation(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	nudge := &fakeNudge{}
	rt.reconcileNudger = nudge
	const path = "/api/v1/settings/ingress/cloudflare"

	rec := rsDo(t, rt, cookie, http.MethodGet, path, "")
	var got cloudflarePresetResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || got.Preset != "none" || got.Ranges == 0 {
		t.Fatalf("get = %d %s", rec.Code, rec.Body.String())
	}
	if rec = rsDo(t, rt, cookie, http.MethodPut, path, `{"preset":"cloudflare"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed = %d %s", rec.Code, rec.Body.String())
	}
	if rec = rsDo(t, rt, cookie, http.MethodPut, path, `{"preset":"cloudflare","confirm_weakening":true}`); rec.Code != http.StatusOK || nudge.count != 1 {
		t.Fatalf("confirmed = %d %s nudges %d", rec.Code, rec.Body.String(), nudge.count)
	}
	// Back to none is stronger and needs no confirmation.
	if rec = rsDo(t, rt, cookie, http.MethodPut, path, `{"preset":"none"}`); rec.Code != http.StatusOK {
		t.Fatalf("off = %d %s", rec.Code, rec.Body.String())
	}
	if rec = rsDo(t, rt, cookie, http.MethodPut, path, `{"preset":"bogus"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus = %d", rec.Code)
	}
}
