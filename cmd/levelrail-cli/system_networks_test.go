package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SystemNetworks(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			var b map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&b)
			gotBody = r.URL.Path
			_ = json.NewEncoder(w).Encode(apiclient.SystemNetworksPrune{DryRun: b["dry_run"], Candidates: []string{"p-app-gone"}, Removed: []string{}})
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.SystemNetworks{
			Pool: apiclient.NetworkPool{CIDR: "10.88.0.0/14", Enabled: true, Capacity: 1024, InUse: 2, Percent: 0},
			Networks: []apiclient.SystemNetwork{
				{Name: "p-app-live", OwnerKind: "app", OwnerName: "live", Reason: "owner_exists"},
				{Name: "p-app-gone", OwnerKind: "app", OwnerName: "gone", Orphaned: true, Removable: true, Reason: "removable"},
			},
			OrphanCount: 1,
		})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	if got := run("levelrail-cli-test", []string{"system", "networks", "--orphans", "--api-url", srv.URL}, &out, &errOut, envMap()); got != exitOK {
		t.Fatalf("exit %d %q", got, errOut.String())
	}
	if !strings.Contains(out.String(), "p-app-gone") || strings.Contains(out.String(), "p-app-live") {
		t.Errorf("orphans filter: %q", out.String())
	}

	out.Reset()
	errOut.Reset()
	if got := run("levelrail-cli-test", []string{"system", "networks", "prune", "--api-url", srv.URL}, &out, &errOut, envMap()); got != exitValidation {
		t.Errorf("prune without flag exit %d", got)
	}
	if got := run("levelrail-cli-test", []string{"system", "networks", "prune", "--dry-run", "--api-url", srv.URL}, &out, &errOut, envMap()); got != exitOK {
		t.Fatalf("dry-run exit %d %q", got, errOut.String())
	}
	if gotBody != "/api/v1/system/networks/prune" || !strings.Contains(out.String(), "would remove 1") {
		t.Errorf("path %q out %q", gotBody, out.String())
	}
}
