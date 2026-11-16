package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
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
