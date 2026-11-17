package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleDestroyNodeProvision(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	if err := db.SaveNodeProvision(context.Background(), store.NodeProvision{
		ID: "npv_d1", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-d1", Role: "general",
		Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-777", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	do := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/node-provisions/"+id, ""))
		return rec
	}

	if rec := do("npv_missing"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d", rec.Code)
	}
	if len(fake.deletedIDs) != 0 {
		t.Fatalf("provider called for an unknown id: %v", fake.deletedIDs)
	}

	if rec := do("npv_d1"); rec.Code != http.StatusOK {
		t.Fatalf("destroy status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedIDs) != 1 || fake.deletedIDs[0] != "srv-777" {
		t.Fatalf("deletedIDs = %v, want only the stored server id", fake.deletedIDs)
	}
	got, _ := db.GetNodeProvision(context.Background(), "npv_d1")
	if got.Status != store.NodeProvisionStatusDestroyed {
		t.Fatalf("status = %q, want destroyed", got.Status)
	}

	if rec := do("npv_d1"); rec.Code != http.StatusOK {
		t.Fatalf("second destroy status = %d", rec.Code)
	}
	if len(fake.deletedIDs) != 1 {
		t.Fatalf("idempotent destroy called the provider again: %v", fake.deletedIDs)
	}

	fake.deleteErr = &provision.ProviderError{Status: http.StatusInternalServerError, Body: "boom tok"}
	_ = db.SaveNodeProvision(context.Background(), store.NodeProvision{
		ID: "npv_d2", Provider: "hetzner", Name: "web-d2", Role: "general",
		Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-888", CreatedAt: now, UpdatedAt: now,
	})
	rec := do("npv_d2")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("provider failure status = %d", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "boom") {
		t.Fatalf("body leaks provider detail: %s", body)
	}
}

func TestHandleCreateNodeProvision_SpendGuard(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{createServerID: "s1", sizes: []provision.Size{{ID: "cx22", PriceMonthly: "50.00", Currency: "USD"}}}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)
	body := `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-g1","control_plane_addr":"cp.example.com:9443"}`
	setCap := func(raw string) {
		if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyProvisionMaxMonthlyPrice, json.RawMessage(raw), "test"); err != nil {
			t.Fatal(err)
		}
	}
	post := func(b string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", b))
		return rec.Code
	}

	setCap(`"10"`)
	if code := post(body); code != http.StatusConflict {
		t.Fatalf("over price cap status = %d, want 409", code)
	}
	setCap(`""`)
	if code := post(body); code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", code)
	}
	if code := post(`{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-g2","control_plane_addr":"cp.example.com:9443"}`); code != http.StatusConflict {
		t.Fatalf("second create status = %d, want 409", code)
	}
	if fake.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1: refused creates must not reach the provider", fake.createCalls)
	}
}
