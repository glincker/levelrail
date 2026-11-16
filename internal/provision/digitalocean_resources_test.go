package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func doDropletJSON(id int, name, slug string, tags []string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "status": "active", "size_slug": slug, "tags": tags,
		"created_at": "2026-11-01T10:00:00Z", "region": map[string]any{"slug": "fra1"},
		"networks": map[string]any{"v4": []map[string]any{
			{"ip_address": "10.0.0.5", "type": "private"}, {"ip_address": "203.0.113." + strconv.Itoa(id), "type": "public"}}},
	}
}

func TestDigitalOceanListResources_TagsPricesAndPagination(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/droplets":
			if r.URL.Query().Get("per_page") != "200" {
				t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
			}
			if r.URL.Query().Get("page") == "2" {
				_ = json.NewEncoder(w).Encode(map[string]any{"droplets": []any{doDropletJSON(3, "plain", "s-2vcpu-4gb", nil)}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"droplets": []any{
					doDropletJSON(1, "managed", "s-1vcpu-1gb", []string{"levelrail", "levelrail-provision-abc"}),
					doDropletJSON(2, "unknown-size", "s-99", []string{}),
				},
				"links": map[string]any{"pages": map[string]any{"next": srv.URL + "/droplets?per_page=200&page=2"}},
			})
		case "/sizes":
			_ = json.NewEncoder(w).Encode(map[string]any{"sizes": []any{
				map[string]any{"slug": "s-1vcpu-1gb", "price_monthly": 6.0},
				map[string]any{"slug": "s-2vcpu-4gb", "price_monthly": 24.0},
			}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	d := &DigitalOcean{client: newHTTPClient(srv.URL, "test-token")}
	res, err := d.ListResources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("want 3 droplets across pages, got %d", len(res))
	}
	if res[0].ID != "1" || res[0].PublicIPv4 != "203.0.113.1" || res[0].Region != "fra1" || len(res[0].Tags) != 2 {
		t.Errorf("first = %+v", res[0])
	}
	if res[0].EstimatedMonthlyPrice == nil || *res[0].EstimatedMonthlyPrice != 6 {
		t.Errorf("price = %v", res[0].EstimatedMonthlyPrice)
	}
	if res[1].EstimatedMonthlyPrice != nil || res[1].Tags == nil {
		t.Errorf("unknown size must have no price and empty (not nil) tags: %+v", res[1])
	}
	if res[2].EstimatedMonthlyPrice == nil || *res[2].EstimatedMonthlyPrice != 24 {
		t.Errorf("page two price = %v", res[2].EstimatedMonthlyPrice)
	}
}

func TestDigitalOceanListResources_EmptyAccount(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"droplets":[],"sizes":[]}`)) })
	res, err := d.ListResources(context.Background())
	if err != nil || len(res) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestDigitalOceanListResources_DropletsForbiddenIsScopeError(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	_, err := d.ListResources(context.Background())
	se, ok := ScopeFromError("digitalocean", "instances.read", err)
	if !ok || se.Needs != "droplet:read" {
		t.Fatalf("err = %v scope=%+v", err, se)
	}
}

func TestDigitalOceanListResources_SizesForbiddenFallsBackToDropletSize(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sizes" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		dr := doDropletJSON(7, "x", "s-1vcpu-1gb", nil)
		dr["size"] = map[string]any{"price_monthly": 6.0}
		_ = json.NewEncoder(w).Encode(map[string]any{"droplets": []any{dr}})
	})
	res, err := d.ListResources(context.Background())
	if err != nil || len(res) != 1 || res[0].EstimatedMonthlyPrice == nil || *res[0].EstimatedMonthlyPrice != 6 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestDigitalOceanReadBilling(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/customers/my/balance":
			_, _ = fmt.Fprint(w, `{"month_to_date_balance":"23.44","account_balance":"12.23","month_to_date_usage":"11.21","generated_at":"2026-11-16T08:00:00Z"}`)
		case "/customers/my/billing_history":
			_, _ = fmt.Fprint(w, `{"billing_history":[
				{"description":"Payment","amount":"-10.00","type":"Payment","date":"2026-11-10T00:00:00Z"},
				{"description":"Invoice for October","amount":"21.00","invoice_id":"123","invoice_uuid":"u-1","type":"Invoice","date":"2026-11-01T00:00:00Z"},
				{"description":"Invoice for September","amount":"9.00","invoice_id":"122","type":"Invoice","date":"2026-10-01T00:00:00Z"}]}`)
		}
	})
	bal, inv := d.ReadBilling(context.Background())
	if bal.State != SectionOK || bal.MonthToDateUsage.Value != "11.21" || bal.MonthToDateUsage.Source == "" || bal.MonthToDateUsage.DataAsOf == nil || bal.GeneratedAt == nil {
		t.Errorf("balance = %+v", bal)
	}
	if inv.State != SectionOK || inv.InvoiceID != "123" || inv.Amount.Value != "21.00" || inv.Amount.Source == "" {
		t.Errorf("invoice = %+v", inv)
	}
}

func TestDigitalOceanReadBilling_ForbiddenAndErrors(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/customers/my/balance" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("secret provider body"))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	bal, inv := d.ReadBilling(context.Background())
	if bal.State != SectionScopeMissing || bal.Needs != "billing:read" || bal.AccountBalance != nil {
		t.Errorf("balance = %+v", bal)
	}
	if inv.State != SectionUnavailable {
		t.Errorf("invoice = %+v", inv)
	}
}
