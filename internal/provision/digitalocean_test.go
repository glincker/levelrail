package provision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestDigitalOcean(t *testing.T, handler http.HandlerFunc) *DigitalOcean {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &DigitalOcean{client: newHTTPClient(srv.URL, "test-token")}
}

func TestDigitalOcean_ListRegions_SkipsUnavailable(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/regions" {
			t.Errorf("path = %q, want /regions", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"regions": []map[string]any{
				{"slug": "nyc1", "name": "New York 1", "available": true},
				{"slug": "ams2", "name": "Amsterdam 2", "available": false},
			},
		})
	})

	regions, err := d.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 1 || regions[0].ID != "nyc1" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestDigitalOcean_ListSizes_FiltersByRegion(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sizes": []map[string]any{
				{
					"slug": "s-1vcpu-1gb", "memory": 1024, "vcpus": 1, "disk": 25,
					"price_monthly": 5.0, "regions": []string{"nyc1", "nyc3"}, "available": true,
				},
				{
					"slug": "s-2vcpu-2gb", "memory": 2048, "vcpus": 2, "disk": 50,
					"price_monthly": 12.0, "regions": []string{"ams3"}, "available": true,
				},
				{
					"slug": "old-size", "memory": 512, "vcpus": 1, "disk": 10,
					"regions": []string{"nyc1"}, "available": false,
				},
			},
		})
	})

	sizes, err := d.ListSizes(context.Background(), "nyc1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 1 || sizes[0].ID != "s-1vcpu-1gb" || sizes[0].PriceMonthly != "5.00" {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestDigitalOcean_CreateServer(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/droplets" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"droplet": map[string]any{
				"id": 999, "status": "new",
				"networks": map[string]any{
					"v4": []map[string]any{{"ip_address": "5.6.7.8", "type": "public"}},
				},
			},
		})
	})

	id, ip, err := d.CreateServer(context.Background(), CreateOpts{Name: "build-1", Region: "nyc1", Size: "s-1vcpu-1gb"})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if id != "999" || ip != "5.6.7.8" {
		t.Errorf("id=%q ip=%q", id, ip)
	}
}

func TestDigitalOcean_GetServer_StatusMapping(t *testing.T) {
	cases := []struct {
		raw  string
		want ServerStatus
	}{
		{"active", ServerStatusRunning},
		{"new", ServerStatusPending},
		{"archive", ServerStatusError},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"droplet": map[string]any{"id": 1, "status": tc.raw, "networks": map[string]any{}},
				})
			})
			status, _, err := d.GetServer(context.Background(), "1")
			if err != nil {
				t.Fatalf("GetServer: %v", err)
			}
			if status != tc.want {
				t.Errorf("status = %q, want %q", status, tc.want)
			}
		})
	}
}

func TestDigitalOcean_DeleteServer_ErrorResponse(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"id":"not_found","message":"droplet not found"}`))
	})
	err := d.DeleteServer(context.Background(), "404")
	if err == nil {
		t.Fatal("expected an error")
	}
}
