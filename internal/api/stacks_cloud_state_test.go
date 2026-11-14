package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const quarantineCanary = "sealed-state-canary-91c2"

func seedQuarantine(t *testing.T, db *store.DB, stack string) int64 {
	t.Helper()
	id, err := db.InsertCloudStateQuarantine(t.Context(), store.CloudStateQuarantine{
		Account: "oci-test", Stack: stack, Sealed: []byte(quarantineCanary), Fence: 7, Reason: "lease lost", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed quarantine: %v", err)
	}
	return id
}

func TestCloudStateQuarantineListAndResolve(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	id := seedQuarantine(t, db, "shop-prod")
	other := seedQuarantine(t, db, "other-stack")

	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/cloud-state/quarantine", "")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), quarantineCanary) || strings.Contains(rec.Body.String(), "sealed") {
		t.Fatalf("list = %d %s, want 200 without the blob", rec.Code, rec.Body.String())
	}
	var list cloudStateQuarantineList
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/cloud-state/quarantine", ""), &list); code != http.StatusOK ||
		len(list.Quarantine) != 1 || list.Quarantine[0].ID != id || list.Quarantine[0].Account != "oci-test" || list.Quarantine[0].Fence != 7 {
		t.Fatalf("list = %d %+v", code, list)
	}

	path := "/api/v1/stacks/shop-prod/cloud-state/quarantine/"
	for _, c := range []struct{ name, path, body string }{
		{"wrong confirm", path + itoa(id) + "/resolve", `{"confirm":"nope"}`},
		{"no body", path + itoa(id) + "/resolve", ``},
		{"bad id", path + "abc/resolve", `{"confirm":"shop-prod"}`},
	} {
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, c.path, c.body), nil); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", c.name, code)
		}
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, path+itoa(other)+"/resolve", `{"confirm":"shop-prod"}`), nil); code != http.StatusNotFound {
		t.Fatalf("another stack's id = %d, want 404", code)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, path+itoa(id)+"/resolve", `{"confirm":"shop-prod"}`), nil); code != http.StatusOK {
		t.Fatalf("resolve = %d", code)
	}
	left, _ := db.ListCloudStateQuarantine(t.Context(), "oci-test", "shop-prod")
	if len(left) != 0 {
		t.Fatalf("row still unresolved: %+v", left)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, path+itoa(id)+"/resolve", `{"confirm":"shop-prod"}`), nil); code != http.StatusNotFound {
		t.Fatalf("second resolve = %d, want 404", code)
	}
	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == auditStackCloudStateRecover {
			found = true
			if strings.Contains(e.Detail, quarantineCanary) {
				t.Fatalf("audit detail leaks state: %s", e.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("no %s audit row", auditStackCloudStateRecover)
	}
}

func TestCloudStateQuarantineRootOnly(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	id := seedQuarantine(t, db, "shop-prod")
	tok := seedMatrixToken(t, db, "cs-write", []string{AbilityRead, AbilityWrite})
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/stacks/shop-prod/cloud-state/quarantine", ""},
		{http.MethodPost, "/api/v1/stacks/shop-prod/cloud-state/quarantine/" + itoa(id) + "/resolve", `{"confirm":"shop-prod"}`},
	} {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer "+tok)
		if code := iacCall(t, rt, req, nil); code != http.StatusForbidden {
			t.Fatalf("%s %s = %d, want 403", c.method, c.path, code)
		}
	}
	if left, _ := db.ListCloudStateQuarantine(t.Context(), "oci-test", "shop-prod"); len(left) != 1 {
		t.Fatalf("a refused call changed the row")
	}
}
