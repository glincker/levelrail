package provision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const vultrTestKey = "vultr-test-key-123"

func newTestVultr(t *testing.T, handler http.HandlerFunc) *Vultr {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := newHTTPClient(srv.URL, vultrTestKey)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return &Vultr{client: c, key: vultrTestKey}
}

func vultrJSON(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

func TestVultr_CreateServer_SendsBase64UserDataPlanRegionTags(t *testing.T) {
	var got map[string]any
	v := newTestVultr(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+vultrTestKey {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/os":
			vultrJSON(w, map[string]any{"os": []map[string]any{
				{"id": 1, "name": "Ubuntu 22.04 LTS x64"},
				{"id": 2, "name": "Ubuntu 24.04 LTS x64"},
				{"id": 3, "name": "Ubuntu 25.10 x64"},
				{"id": 4, "name": "Debian 12 x64"},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/instances":
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.WriteHeader(http.StatusAccepted)
			vultrJSON(w, map[string]any{"instance": map[string]any{"id": "inst-uuid-0001", "main_ip": "0.0.0.0"}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	id, ip, err := v.CreateServer(context.Background(), CreateOpts{
		Region: "ewr", Size: "vc2-1c-1gb", Name: "web-1", UserData: "#cloud-config\nx: 1", ProvisionID: "npv_1",
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if id != "inst-uuid-0001" || ip != "" {
		t.Errorf("id=%q ip=%q", id, ip)
	}
	if got["region"] != "ewr" || got["plan"] != "vc2-1c-1gb" || got["os_id"] != float64(2) {
		t.Errorf("body = %+v", got)
	}
	raw, _ := base64.StdEncoding.DecodeString(got["user_data"].(string))
	if string(raw) != "#cloud-config\nx: 1" {
		t.Errorf("user_data decoded = %q", raw)
	}
	tags, _ := json.Marshal(got["tags"])
	if string(tags) != `["levelrail","levelrail-provision-npv_1"]` {
		t.Errorf("tags = %s", tags)
	}
}

func TestVultr_GetServer_Status(t *testing.T) {
	cases := []struct {
		status, power, ip string
		want              ServerStatus
		wantIP            string
	}{
		{"pending", "running", "0.0.0.0", ServerStatusPending, ""},
		{"active", "running", "0.0.0.0", ServerStatusPending, ""},
		{"active", "running", "203.0.113.9", ServerStatusRunning, "203.0.113.9"},
		{"active", "stopped", "203.0.113.9", ServerStatusError, "203.0.113.9"},
		{"suspended", "running", "203.0.113.9", ServerStatusError, "203.0.113.9"},
	}
	for _, c := range cases {
		v := newTestVultr(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/instances/inst-uuid-0001" {
				t.Errorf("path = %q", r.URL.Path)
			}
			vultrJSON(w, map[string]any{"instance": map[string]any{"status": c.status, "power_status": c.power, "main_ip": c.ip}})
		})
		st, ip, err := v.GetServer(context.Background(), "inst-uuid-0001")
		if err != nil || st != c.want || ip != c.wantIP {
			t.Errorf("%+v: st=%q ip=%q err=%v", c, st, ip, err)
		}
	}
}

func TestVultr_DeleteServer_IdempotentAndStoredIDOnly(t *testing.T) {
	var deletes []string
	v := newTestVultr(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		deletes = append(deletes, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "gone-uuid-0001") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := v.DeleteServer(context.Background(), "inst-uuid-0001"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := v.DeleteServer(context.Background(), "gone-uuid-0001"); err != nil {
		t.Fatalf("404 must be success: %v", err)
	}
	for _, bad := range []string{"", "../instances", "a/b"} {
		if err := v.DeleteServer(context.Background(), bad); err == nil {
			t.Errorf("id %q must be refused", bad)
		}
	}
	if len(deletes) != 2 || deletes[0] != "/instances/inst-uuid-0001" {
		t.Errorf("deletes = %v", deletes)
	}
}

func TestVultr_Unauthorized_ErrorHasNoBodyOrKey(t *testing.T) {
	v := newTestVultr(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Invalid API key secret-provider-body ` + vultrTestKey + `"}`))
	})
	_, err := v.ListRegions(context.Background())
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "secret-provider-body") || strings.Contains(err.Error(), vultrTestKey) {
		t.Errorf("error leaks: %v", err)
	}
	if se, ok := ScopeFromError("vultr", "regions.read", err); !ok || se.Needs == "" {
		t.Errorf("scope mapping = %v %v", se, ok)
	}
}

func TestVultr_RateLimitRetries(t *testing.T) {
	var calls atomic.Int32
	v := newTestVultr(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		vultrJSON(w, map[string]any{"regions": []map[string]any{{"id": "ewr", "city": "Piscataway", "country": "US"}}})
	})
	regions, err := v.ListRegions(context.Background())
	if err != nil || len(regions) != 1 || regions[0].ID != "ewr" || calls.Load() != 3 {
		t.Fatalf("regions=%+v err=%v calls=%d", regions, err, calls.Load())
	}
}

func TestVultr_RateLimitExhaustedHasNoBody(t *testing.T) {
	v := newTestVultr(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("secret-provider-body"))
	})
	_, err := v.ListRegions(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret-provider-body") {
		t.Fatalf("err = %v", err)
	}
}

func TestVultr_ListSizes_CarriesRegionsPriceAndFilters(t *testing.T) {
	var pages atomic.Int32
	v := newTestVultr(t, func(w http.ResponseWriter, r *http.Request) {
		pages.Add(1)
		if r.URL.Query().Get("cursor") == "c2" {
			vultrJSON(w, map[string]any{"plans": []map[string]any{
				{"id": "vc2-2c-4gb", "vcpu_count": 2, "ram": 4096, "disk": 80, "monthly_cost": 20, "locations": []string{"ams"}},
			}})
			return
		}
		vultrJSON(w, map[string]any{
			"plans": []map[string]any{
				{"id": "vc2-1c-1gb", "vcpu_count": 1, "ram": 1024, "disk": 25, "monthly_cost": 5.5, "locations": []string{"ewr", "ams"}},
			},
			"meta": map[string]any{"links": map[string]any{"next": "c2"}},
		})
	})
	all, err := v.ListSizes(context.Background(), "")
	if err != nil || len(all) != 2 || pages.Load() != 2 {
		t.Fatalf("all=%+v err=%v pages=%d", all, err, pages.Load())
	}
	if all[0].PriceMonthly != "5.50" || all[0].Currency != "USD" || all[0].Memory != 1024 || len(all[0].Regions) != 2 {
		t.Errorf("size = %+v", all[0])
	}
	ewr, _ := v.ListSizes(context.Background(), "ewr")
	if len(ewr) != 1 || ewr[0].ID != "vc2-1c-1gb" {
		t.Errorf("ewr sizes = %+v", ewr)
	}
}

func TestVultr_ProbeCapabilities_Granted(t *testing.T) {
	var paths atomic.Int32
	v := newTestVultr(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("probe used %s", r.Method)
		}
		paths.Add(1)
		_, _ = w.Write([]byte("{}"))
	})
	pc := v.ProbeCapabilities(context.Background(), 3)
	if c := capByID(t, pc, "instances.read"); c.State != CapabilityGranted || c.Evidence != EvidenceProbeOK {
		t.Errorf("instances.read = %+v", c)
	}
	if c := capByID(t, pc, "instances.create"); c.State != CapabilityUnknown {
		t.Errorf("write must stay unknown: %+v", c)
	}
	if paths.Load() != 4 || pc.LiveCheckError != "" {
		t.Errorf("probes=%d err=%q", paths.Load(), pc.LiveCheckError)
	}
}

func TestVultr_ProbeCapabilities_Missing(t *testing.T) {
	v := newTestVultr(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/instances" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("secret-provider-body"))
			return
		}
		_, _ = w.Write([]byte("{}"))
	})
	pc := v.ProbeCapabilities(context.Background(), 2)
	c := capByID(t, pc, "instances.read")
	if c.State != CapabilityMissing || c.Evidence != EvidenceProbe403 || c.Needs == "" {
		t.Errorf("instances.read = %+v", c)
	}
	if capByID(t, pc, "account.read").State != CapabilityGranted {
		t.Errorf("account should stay granted")
	}
}
