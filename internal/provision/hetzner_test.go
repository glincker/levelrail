package provision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestHetzner(t *testing.T, handler http.HandlerFunc) *Hetzner {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Hetzner{client: newHTTPClient(srv.URL, "test-token")}
}

func TestHetzner_ListRegions(t *testing.T) {
	h := newTestHetzner(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/locations" {
			t.Errorf("path = %q, want /locations", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"locations": []map[string]any{
				{"name": "fsn1", "description": "Falkenstein"},
				{"name": "nbg1", "description": "Nuremberg"},
			},
		})
	})

	regions, err := h.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 || regions[0].ID != "fsn1" || regions[0].Name != "Falkenstein" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestHetzner_ListRegions_FollowsPagination(t *testing.T) {
	var pages []string
	h := newTestHetzner(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		switch page {
		case "1", "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"locations": []map[string]any{{"name": "fsn1", "description": "Falkenstein"}},
				"meta":      map[string]any{"pagination": map[string]any{"next_page": 2}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"locations": []map[string]any{{"name": "nbg1", "description": "Nuremberg"}},
				"meta":      map[string]any{"pagination": map[string]any{"next_page": 0}},
			})
		}
	})

	regions, err := h.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("fetched %d pages, want 2 (pages=%v)", len(pages), pages)
	}
	if len(regions) != 2 || regions[0].ID != "fsn1" || regions[1].ID != "nbg1" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestHetzner_ListSizes_FiltersByRegionAvailability(t *testing.T) {
	h := newTestHetzner(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"server_types": []map[string]any{
				{
					"name": "cx22", "description": "cx22", "cores": 2, "memory": 4.0, "disk": 40,
					"prices": []map[string]any{
						{"location": "fsn1", "price_monthly": map[string]any{"gross": "4.90"}},
					},
				},
				{
					"name": "cx32", "description": "cx32", "cores": 4, "memory": 8.0, "disk": 80,
					"prices": []map[string]any{
						{"location": "nbg1", "price_monthly": map[string]any{"gross": "8.90"}},
					},
				},
				{"name": "old1", "description": "deprecated", "deprecated": true},
			},
		})
	})

	sizes, err := h.ListSizes(context.Background(), "fsn1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 1 || sizes[0].ID != "cx22" || sizes[0].PriceMonthly != "4.90" {
		t.Errorf("sizes = %+v", sizes)
	}
	if sizes[0].Memory != 4096 {
		t.Errorf("Memory = %d, want 4096", sizes[0].Memory)
	}
}

func TestHetzner_CreateServer(t *testing.T) {
	h := newTestHetzner(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/servers" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "web-1" {
			t.Errorf("request name = %v", body["name"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"server": map[string]any{
				"id": 12345, "status": "initializing",
				"public_net": map[string]any{"ipv4": map[string]any{"ip": "1.2.3.4"}},
			},
		})
	})

	id, ip, err := h.CreateServer(context.Background(), CreateOpts{Name: "web-1", Region: "fsn1", Size: "cx22"})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if id != "12345" || ip != "1.2.3.4" {
		t.Errorf("id=%q ip=%q", id, ip)
	}
}

func TestHetzner_GetServer_StatusMapping(t *testing.T) {
	cases := []struct {
		raw  string
		want ServerStatus
	}{
		{"running", ServerStatusRunning},
		{"initializing", ServerStatusPending},
		{"off", ServerStatusError},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			h := newTestHetzner(t, func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"server": map[string]any{"id": 1, "status": tc.raw},
				})
			})
			status, _, err := h.GetServer(context.Background(), "1")
			if err != nil {
				t.Fatalf("GetServer: %v", err)
			}
			if status != tc.want {
				t.Errorf("status = %q, want %q", status, tc.want)
			}
		})
	}
}

func TestHetzner_GetServer_NotFound(t *testing.T) {
	h := newTestHetzner(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"server not found"}}`))
	})
	_, _, err := h.GetServer(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestHetzner_DeleteServer(t *testing.T) {
	var called bool
	h := newTestHetzner(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete || r.URL.Path != "/servers/999" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"action": map[string]any{"id": 1}})
	})
	if err := h.DeleteServer(context.Background(), "999"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}
