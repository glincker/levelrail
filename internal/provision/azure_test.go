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

func newTestAzure(t *testing.T, handler http.HandlerFunc) *Azure {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cred := azureCredential{
		TenantID: "tenant", ClientID: "client", ClientSecret: "secret",
		SubscriptionID: "sub-1", ResourceGroup: "rg-1",
	}
	ts := fakeTokenSource{token: &oauth2.Token{AccessToken: "fake-access-token"}}
	return newAzure(cred, ts, srv.URL)
}

func TestAzure_ParseCredential_Valid(t *testing.T) {
	raw := `{"tenant_id":"t","client_id":"c","client_secret":"s","subscription_id":"sub","resource_group":"rg"}`
	cred, err := parseAzureCredential(raw)
	if err != nil {
		t.Fatalf("parseAzureCredential: %v", err)
	}
	if cred.TenantID != "t" || cred.ClientID != "c" || cred.ClientSecret != "s" || cred.SubscriptionID != "sub" || cred.ResourceGroup != "rg" {
		t.Errorf("cred = %+v", cred)
	}
}

func TestAzure_ParseCredential_ValidationError(t *testing.T) {
	cases := []string{
		`not json`,
		`{}`,
		`{"tenant_id":"t"}`,
		`{"tenant_id":"t","client_id":"c","client_secret":"s","subscription_id":"sub"}`,
		// Neither client_secret nor federated_token_file: still invalid.
		`{"tenant_id":"t","client_id":"c","subscription_id":"sub","resource_group":"rg"}`,
	}
	for _, raw := range cases {
		if _, err := parseAzureCredential(raw); err == nil {
			t.Errorf("parseAzureCredential(%q): expected an error", raw)
		}
	}
}

func TestAzure_ParseCredential_FederatedTokenFileSatisfiesAuth(t *testing.T) {
	raw := `{"tenant_id":"t","client_id":"c","federated_token_file":"/var/run/secrets/azure/token","subscription_id":"sub","resource_group":"rg"}`
	cred, err := parseAzureCredential(raw)
	if err != nil {
		t.Fatalf("parseAzureCredential: %v", err)
	}
	if cred.FederatedTokenFile != "/var/run/secrets/azure/token" || cred.ClientSecret != "" {
		t.Errorf("cred = %+v", cred)
	}
}

func TestAzure_ListRegions_SkipsNonPhysical(t *testing.T) {
	a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/locations") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{"name": "eastus", "displayName": "East US", "metadata": map[string]any{"regionType": "Physical"}},
				{"name": "eastus-dr", "displayName": "East US DR", "metadata": map[string]any{"regionType": "Logical"}},
			},
		})
	})
	regions, err := a.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 1 || regions[0].ID != "eastus" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestAzure_ListSizes(t *testing.T) {
	a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/vmSizes") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{"name": "Standard_B1s", "numberOfCores": 1, "memoryInMB": 1024, "osDiskSizeInMB": 30720},
			},
		})
	})
	sizes, err := a.ListSizes(context.Background(), "eastus")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 1 || sizes[0].ID != "Standard_B1s" || sizes[0].VCPUs != 1 || sizes[0].Memory != 1024 || sizes[0].Disk != 30 {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestAzure_ListSizes_RequiresRegion(t *testing.T) {
	a := newTestAzure(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call the API without a region")
	})
	if _, err := a.ListSizes(context.Background(), ""); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAzure_CreateServer(t *testing.T) {
	var methods, paths []string
	rgCalls := 0
	a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "/virtualNetworks/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"properties": map[string]any{
					"subnets": []map[string]any{{"id": "/subnets/default"}},
				},
			})
		case strings.Contains(r.URL.Path, "/publicIPAddresses/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "/pips/build-1-pip",
				"properties": map[string]any{"ipAddress": "20.1.2.3"},
			})
		case strings.Contains(r.URL.Path, "/networkInterfaces/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "/nics/build-1-nic"})
		case strings.Contains(r.URL.Path, "/virtualMachines/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"properties": map[string]any{"provisioningState": "Creating"}})
		case strings.Contains(r.URL.Path, "/resourcegroups/"):
			rgCalls++
			if r.Method == http.MethodGet {
				// Not found yet: ensureResourceGroup must fall through to
				// a PUT to create it.
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	id, ip, err := a.CreateServer(context.Background(), CreateOpts{
		Name: "build-1", Region: "eastus", Size: "Standard_B1s", UserData: "#cloud-init\n",
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if id != "build-1" || ip != "20.1.2.3" {
		t.Errorf("id=%q ip=%q", id, ip)
	}
	if rgCalls != 2 {
		t.Errorf("resource group calls = %d, want 2 (GET then PUT, group didn't exist yet)", rgCalls)
	}
	if len(paths) != 6 {
		t.Fatalf("made %d calls, want 6 (resource group GET+PUT, vnet, public ip, nic, vm): %v", len(paths), paths)
	}
	for i, p := range paths {
		if strings.Contains(p, "/resourcegroups/") {
			continue
		}
		if methods[i] != http.MethodPut {
			t.Errorf("method for %s = %s, want PUT", p, methods[i])
		}
	}
}

// TestAzure_CreateServer_ExistingResourceGroupNotRelocated covers the fix
// for ensureResourceGroup unconditionally PUTing the resource group even
// when it already exists in a different Azure location than the one being
// provisioned into: Azure rejects that as an attempted relocation, so
// CreateServer must not attempt it once the group is confirmed to exist.
func TestAzure_CreateServer_ExistingResourceGroupNotRelocated(t *testing.T) {
	rgPUTCalls := 0
	a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/resourcegroups/"):
			if r.Method == http.MethodPut {
				rgPUTCalls++
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"location": "westeurope"})
		case strings.Contains(r.URL.Path, "/virtualNetworks/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"properties": map[string]any{"subnets": []map[string]any{{"id": "/subnets/default"}}},
			})
		case strings.Contains(r.URL.Path, "/publicIPAddresses/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "/pips/build-1-pip", "properties": map[string]any{"ipAddress": "20.1.2.3"},
			})
		case strings.Contains(r.URL.Path, "/networkInterfaces/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "/nics/build-1-nic"})
		case strings.Contains(r.URL.Path, "/virtualMachines/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"properties": map[string]any{"provisioningState": "Creating"}})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	if _, _, err := a.CreateServer(context.Background(), CreateOpts{
		Name: "build-1", Region: "eastus", Size: "Standard_B1s", UserData: "x",
	}); err != nil {
		t.Fatalf("CreateServer: %v (an existing group in a different region must not block creation)", err)
	}
	if rgPUTCalls != 0 {
		t.Errorf("rgPUTCalls = %d, want 0 (must not attempt to relocate an existing resource group)", rgPUTCalls)
	}
}

// TestAzure_CreateServer_NICFailureRollsBackPublicIP covers the fix for a
// partial Azure create leaving an untracked, billable public IP: if NIC
// creation fails after the public IP was already created, CreateServer
// must delete that public IP before returning.
func TestAzure_CreateServer_NICFailureRollsBackPublicIP(t *testing.T) {
	pipDeleteCalls := 0
	a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/resourcegroups/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/virtualNetworks/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"properties": map[string]any{"subnets": []map[string]any{{"id": "/subnets/default"}}},
			})
		case strings.Contains(r.URL.Path, "/publicIPAddresses/"):
			if r.Method == http.MethodDelete {
				pipDeleteCalls++
				w.WriteHeader(http.StatusOK)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "/pips/build-1-pip", "properties": map[string]any{"ipAddress": "20.1.2.3"},
			})
		case strings.Contains(r.URL.Path, "/networkInterfaces/"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"nic create failed"}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	if _, _, err := a.CreateServer(context.Background(), CreateOpts{
		Name: "build-1", Region: "eastus", Size: "Standard_B1s", UserData: "x",
	}); err == nil {
		t.Fatal("CreateServer: want an error when NIC creation fails")
	}
	if pipDeleteCalls != 1 {
		t.Errorf("pipDeleteCalls = %d, want 1", pipDeleteCalls)
	}
}

// TestAzure_CreateServer_VMFailureRollsBackNICAndPublicIP is the same
// coverage one step later: a VM creation failure must roll back both the
// NIC and the public IP it and the VM would otherwise leave orphaned.
func TestAzure_CreateServer_VMFailureRollsBackNICAndPublicIP(t *testing.T) {
	var deletedPaths []string
	a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/resourcegroups/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/virtualNetworks/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"properties": map[string]any{"subnets": []map[string]any{{"id": "/subnets/default"}}},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/publicIPAddresses/"):
			deletedPaths = append(deletedPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/networkInterfaces/"):
			deletedPaths = append(deletedPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/publicIPAddresses/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "/pips/build-1-pip", "properties": map[string]any{"ipAddress": "20.1.2.3"},
			})
		case strings.Contains(r.URL.Path, "/networkInterfaces/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "/nics/build-1-nic"})
		case strings.Contains(r.URL.Path, "/virtualMachines/"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"vm create failed"}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	if _, _, err := a.CreateServer(context.Background(), CreateOpts{
		Name: "build-1", Region: "eastus", Size: "Standard_B1s", UserData: "x",
	}); err == nil {
		t.Fatal("CreateServer: want an error when VM creation fails")
	}
	if len(deletedPaths) != 2 {
		t.Fatalf("deleted %d resources, want 2 (nic, public ip): %v", len(deletedPaths), deletedPaths)
	}
	if !strings.Contains(deletedPaths[0], "/networkInterfaces/") || !strings.Contains(deletedPaths[1], "/publicIPAddresses/") {
		t.Errorf("delete order = %v, want nic before public ip", deletedPaths)
	}
}

func TestAzure_CreateServer_RequiresFields(t *testing.T) {
	a := newTestAzure(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call the API with missing required fields")
	})
	if _, _, err := a.CreateServer(context.Background(), CreateOpts{Name: "build-1"}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAzure_GetServer_StatusMapping(t *testing.T) {
	cases := []struct {
		raw  string
		want ServerStatus
	}{
		{"Succeeded", ServerStatusRunning},
		{"Creating", ServerStatusPending},
		{"Failed", ServerStatusError},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/publicIPAddresses/") {
					_ = json.NewEncoder(w).Encode(map[string]any{"properties": map[string]any{"ipAddress": "1.2.3.4"}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"properties": map[string]any{"provisioningState": tc.raw}})
			})
			status, ip, err := a.GetServer(context.Background(), "build-1")
			if err != nil {
				t.Fatalf("GetServer: %v", err)
			}
			if status != tc.want {
				t.Errorf("status = %q, want %q", status, tc.want)
			}
			if ip != "1.2.3.4" {
				t.Errorf("ip = %q", ip)
			}
		})
	}
}

func TestAzure_DeleteServer_ErrorResponse(t *testing.T) {
	a := newTestAzure(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"NotFound"}}`))
	})
	if err := a.DeleteServer(context.Background(), "build-1"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAzure_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("must not call the management API when token acquisition fails")
	}))
	t.Cleanup(srv.Close)
	cred := azureCredential{TenantID: "t", ClientID: "c", ClientSecret: "s", SubscriptionID: "sub", ResourceGroup: "rg"}
	ts := fakeTokenSource{err: errors.New("invalid_client: bad client secret")}
	a := newAzure(cred, ts, srv.URL)

	_, err := a.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("error = %v, want it to mention invalid_client", err)
	}
}
