package api

import (
	"net/http"
	"testing"
)

func TestAppsShowManagingStack(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	if code, out := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil); code != http.StatusOK {
		t.Fatalf("stack apply = %d %+v", code, out)
	}
	var one struct {
		ManagedByStack string `json:"managed_by_stack"`
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""), &one); code != http.StatusOK || one.ManagedByStack != "shop-prod" {
		t.Fatalf("get app = %d %+v", code, one)
	}
	var list []struct {
		Name           string `json:"name"`
		ManagedByStack string `json:"managed_by_stack"`
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""), &list); code != http.StatusOK {
		t.Fatalf("list apps = %d", code)
	}
	found := false
	for _, a := range list {
		if a.Name == "web" {
			found = true
			if a.ManagedByStack != "shop-prod" {
				t.Fatalf("list managed_by_stack = %q", a.ManagedByStack)
			}
		}
	}
	if !found {
		t.Fatal("web missing from list")
	}
}
