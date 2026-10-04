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

func newTestRouterForFirewallRules(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithFirewallRequiredPorts([]int{8080, 9443, 80, 443})), db
}

func TestFirewallRuleRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouterForFirewallRules(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/firewall-rules", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateFirewallRule_Success(t *testing.T) {
	rt, db := newTestRouterForFirewallRules(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"port":9000,"protocol":"tcp","source_cidr":"10.0.0.0/24","action":"allow","label":"vpn"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/firewall-rules", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got firewallRuleResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID == "" || !strings.HasPrefix(got.ID, "fwrule_") {
		t.Errorf("ID = %q, want a fwrule_-prefixed id", got.ID)
	}
	if got.Port != 9000 || got.Action != "allow" || got.SourceCIDR != "10.0.0.0/24" {
		t.Errorf("response = %+v, want it to match the request", got)
	}
}

func TestHandleCreateFirewallRule_InvalidPort(t *testing.T) {
	rt, db := newTestRouterForFirewallRules(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/firewall-rules", `{"port":0}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateFirewallRule_InvalidCIDR(t *testing.T) {
	rt, db := newTestRouterForFirewallRules(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/firewall-rules", `{"port":9000,"source_cidr":"not-a-cidr"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// Deny on the management API port (8080, required per
// WithFirewallRequiredPorts above) must be refused with a clear error,
// never silently created: this is the lockout-prevention rule.
func TestHandleCreateFirewallRule_RefusesLockout(t *testing.T) {
	rt, db := newTestRouterForFirewallRules(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/firewall-rules", `{"port":8080,"action":"deny"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "required by the control plane") {
		t.Errorf("body = %s, want a clear lock-out explanation", rec.Body.String())
	}

	rules, err := db.ListFirewallRules(context.Background())
	if err != nil {
		t.Fatalf("ListFirewallRules() error = %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected the refused rule to never reach the store, got %+v", rules)
	}
}

// Restricting the agent gRPC port (9443) to one source CIDR is just as
// unsafe as a deny: it implicitly cuts off every agent outside that
// CIDR.
func TestHandleCreateFirewallRule_RefusesCIDRScopedRequiredPort(t *testing.T) {
	rt, db := newTestRouterForFirewallRules(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/firewall-rules", `{"port":9443,"action":"allow","source_cidr":"10.0.0.0/24"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleListAndDeleteFirewallRule(t *testing.T) {
	rt, db := newTestRouterForFirewallRules(t)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/firewall-rules", `{"port":9000,"action":"deny"}`))
	var created firewallRuleResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/firewall-rules", ""))
	var list []firewallRuleResource
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want exactly the created rule", list)
	}

	deleteRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deleteRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/firewall-rules/"+created.ID, ""))
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	missingDeleteRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(missingDeleteRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/firewall-rules/fwrule_missing", ""))
	if missingDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want %d", missingDeleteRec.Code, http.StatusNotFound)
	}
}
