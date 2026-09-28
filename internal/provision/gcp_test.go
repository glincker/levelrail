package provision

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func newTestGCP(t *testing.T, handler http.HandlerFunc) *GCP {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ts := fakeTokenSource{token: &oauth2.Token{AccessToken: "fake-access-token"}}
	return newGCP("proj-1", ts, srv.URL)
}

func TestGCP_ParseProjectID(t *testing.T) {
	id, err := parseGCPProjectID(`{"project_id":"proj-1","type":"service_account"}`)
	if err != nil {
		t.Fatalf("parseGCPProjectID: %v", err)
	}
	if id != "proj-1" {
		t.Errorf("id = %q", id)
	}
}

func TestGCP_ParseProjectID_ValidationError(t *testing.T) {
	cases := []string{`not json`, `{}`, `{"type":"service_account"}`}
	for _, raw := range cases {
		if _, err := parseGCPProjectID(raw); err == nil {
			t.Errorf("parseGCPProjectID(%q): expected an error", raw)
		}
	}
}

func TestGCP_ListRegions_SkipsDownZones(t *testing.T) {
	g := newTestGCP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones" {
			t.Errorf("path = %q, want /zones", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"name": "us-central1-a", "status": "UP"},
				{"name": "us-central1-f", "status": "DOWN"},
			},
		})
	})
	zones, err := g.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(zones) != 1 || zones[0].ID != "us-central1-a" {
		t.Errorf("zones = %+v", zones)
	}
}

func TestGCP_ListRegions_FollowsPagination(t *testing.T) {
	var paths []string
	g := newTestGCP(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		if r.URL.Query().Get("pageToken") == "next" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "us-east1-b", "status": "UP"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":         []map[string]any{{"name": "us-central1-a", "status": "UP"}},
			"nextPageToken": "next",
		})
	})
	zones, err := g.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("made %d requests, want 2: %v", len(paths), paths)
	}
	if len(zones) != 2 {
		t.Errorf("zones = %+v", zones)
	}
}

func TestGCP_ListSizes_SkipsDeprecated(t *testing.T) {
	g := newTestGCP(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/machineTypes") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"name": "e2-micro", "guestCpus": 2, "memoryMb": 1024},
				{"name": "old-type", "guestCpus": 1, "memoryMb": 512, "deprecated": map[string]any{"state": "OBSOLETE"}},
			},
		})
	})
	sizes, err := g.ListSizes(context.Background(), "us-central1-a")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 1 || sizes[0].ID != "e2-micro" || sizes[0].VCPUs != 2 {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestGCP_ListSizes_RequiresZone(t *testing.T) {
	g := newTestGCP(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call the API without a zone")
	})
	if _, err := g.ListSizes(context.Background(), ""); err == nil {
		t.Fatal("expected an error")
	}
}

func TestGCP_CreateServer(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	g := newTestGCP(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "RUNNING"})
	})
	id, ip, err := g.CreateServer(context.Background(), CreateOpts{
		Name: "build-1", Region: "us-central1-a", Size: "e2-micro", UserData: "#cloud-init\n",
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if id != "us-central1-a/build-1" || ip != "" {
		t.Errorf("id=%q ip=%q", id, ip)
	}
	if gotMethod != http.MethodPost || gotPath != "/zones/us-central1-a/instances" {
		t.Errorf("method=%s path=%q", gotMethod, gotPath)
	}
	if gotBody["name"] != "build-1" {
		t.Errorf("body name = %v", gotBody["name"])
	}
}

func TestGCP_CreateServer_RequiresFields(t *testing.T) {
	g := newTestGCP(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call the API with missing required fields")
	})
	if _, _, err := g.CreateServer(context.Background(), CreateOpts{Name: "build-1"}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestGCP_CreateServer_InlineOperationError(t *testing.T) {
	g := newTestGCP(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"errors": []map[string]any{{"message": "quota exceeded"}}},
		})
	})
	_, _, err := g.CreateServer(context.Background(), CreateOpts{Name: "build-1", Region: "us-central1-a", Size: "e2-micro"})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("err = %v, want it to mention quota exceeded", err)
	}
}

func TestGCP_GetServer_StatusMapping(t *testing.T) {
	cases := []struct {
		raw  string
		want ServerStatus
	}{
		{"RUNNING", ServerStatusRunning},
		{"PROVISIONING", ServerStatusPending},
		{"TERMINATED", ServerStatusError},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			g := newTestGCP(t, func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": tc.raw,
					"networkInterfaces": []map[string]any{
						{"accessConfigs": []map[string]any{{"natIP": "34.1.1.1"}}},
					},
				})
			})
			status, ip, err := g.GetServer(context.Background(), "us-central1-a/build-1")
			if err != nil {
				t.Fatalf("GetServer: %v", err)
			}
			if status != tc.want {
				t.Errorf("status = %q, want %q", status, tc.want)
			}
			if ip != "34.1.1.1" {
				t.Errorf("ip = %q", ip)
			}
		})
	}
}

func TestGCP_GetServer_MalformedID(t *testing.T) {
	g := newTestGCP(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call the API with a malformed id")
	})
	if _, _, err := g.GetServer(context.Background(), "no-slash"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestGCP_DeleteServer_ErrorResponse(t *testing.T) {
	g := newTestGCP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/zones/us-central1-a/instances/build-1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	if err := g.DeleteServer(context.Background(), "us-central1-a/build-1"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestGCP_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call the Compute API when token acquisition fails")
	}))
	t.Cleanup(srv.Close)
	ts := fakeTokenSource{err: errors.New("invalid_grant: bad private key")}
	g := newGCP("proj-1", ts, srv.URL)

	_, err := g.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error = %v, want it to mention invalid_grant", err)
	}
}
