package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSupplyChainDefault_AppliesWhenAppHasNoPolicy(t *testing.T) {
	rt, db, _, _ := newSupplyChainTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	if rec := scCall(t, rt, cookie, http.MethodPut, "/api/v1/settings/supply-chain", `{"scan_gate":"warn"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("gate without scanning status = %d, want 400", rec.Code)
	}
	rec := scCall(t, rt, cookie, http.MethodPut, "/api/v1/settings/supply-chain", `{"scan_enabled":true,"scan_gate":"block_on_critical"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put default: %d %s", rec.Code, rec.Body.String())
	}
	rec = scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/supply-chain", "")
	var got supplyChainSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.ScanEnabled || got.ScanGate != "block_on_critical" {
		t.Errorf("app without policy = %+v (%v), want the instance default", got, err)
	}
}

func TestSupplyChainDefault_AppPolicyWins(t *testing.T) {
	rt, db, _, _ := newSupplyChainTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	scCall(t, rt, cookie, http.MethodPut, "/api/v1/settings/supply-chain", `{"scan_enabled":true,"scan_gate":"block_on_critical"}`)
	scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{"scan_enabled":false,"scan_gate":"off"}`)

	rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/supply-chain", "")
	var got supplyChainSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ScanEnabled || got.ScanGate != "off" {
		t.Errorf("explicit app policy = %+v (%v), want scanning off", got, err)
	}
	rec = scCall(t, rt, cookie, http.MethodGet, "/api/v1/settings/supply-chain", "")
	var def supplyChainDefaultResource
	if err := json.Unmarshal(rec.Body.Bytes(), &def); err != nil || !def.ScanEnabled || def.ScanGate != "block_on_critical" {
		t.Errorf("default = %+v (%v), want unchanged", def, err)
	}
}
