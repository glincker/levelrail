// Whole-chain proof over a real server and store: unit/handler tests
// already cover rendering and validation with fakes, not whether the
// real mux and auth middleware actually serve this end to end.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eStatusPageAdminUsername = "e2e-status-page-admin"
	e2eStatusPageAdminPassword = "e2e-status-page-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eStatusPageAppName       = "levelrail-test-e2e-status-page-app"
)

func TestStatusPage_Live_EnableAuthorAndServePublicly(t *testing.T) {
	svcStore := openLiveStore(t)
	ctx := context.Background()

	// A real desired-service row is all status_page.go's KindApp
	// validation (validateStatusComponent) needs: it checks the app
	// exists via GetDesiredService, not that a container is running.
	if err := svcStore.SaveDesiredService(ctx, store.DesiredService{Name: e2eStatusPageAppName, Image: "levelrail/e2e-status-page:1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore, api.WithStatusPage(svcStore, statuspage.Config{}, 1000))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, svcStore, e2eStatusPageAdminUsername, e2eStatusPageAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, ts.URL, e2eStatusPageAdminUsername, e2eStatusPageAdminPassword)
	anon := &http.Client{Timeout: e2eHTTPTimeout}

	// Step 1: before any admin configuration, the public routes 404 for
	// an unauthenticated reader. The feature is opt-in by default.
	for _, path := range []string{"/public/status", "/public/status.json", "/public/status.rss"} {
		if status, _ := requestJSON(t, anon, http.MethodGet, ts.URL+path, ""); status != http.StatusNotFound {
			t.Fatalf("GET %s before enabling: status = %d, want %d", path, status, http.StatusNotFound)
		}
	}

	// Step 2: a real admin session enables the page and adds a real app
	// component, over the real authenticated management API.
	status, body := requestJSON(t, admin, http.MethodPut, ts.URL+"/api/v1/status-page",
		`{"enabled":true,"title":"E2E Status","description":"Live service health"}`)
	if status != http.StatusOK {
		t.Fatalf("enable status page: status = %d, body = %s", status, body)
	}

	status, body = requestJSON(t, admin, http.MethodPost, ts.URL+"/api/v1/status-page/components",
		`{"kind":"app","target":"`+e2eStatusPageAppName+`","display_name":"Checkout API"}`)
	if status != http.StatusCreated {
		t.Fatalf("create component: status = %d, body = %s", status, body)
	}
	var comp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &comp); err != nil || comp.ID == "" {
		t.Fatalf("decode component response: err = %v, body = %s", err, body)
	}

	// Step 3: the admin authors a real incident against that component.
	status, body = requestJSON(t, admin, http.MethodPost, ts.URL+"/api/v1/status-page/incidents",
		`{"kind":"incident","title":"Checkout API degraded","impact":"minor","body":"We are investigating elevated latency.","component_ids":["`+comp.ID+`"]}`)
	if status != http.StatusCreated {
		t.Fatalf("create incident: status = %d, body = %s", status, body)
	}
	var inc struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &inc); err != nil || inc.ID == "" {
		t.Fatalf("decode incident response: err = %v, body = %s", err, body)
	}

	// Step 4: a genuinely unauthenticated client (no login, no cookie
	// jar shared with admin) reads each public format and sees the real
	// component name and incident title.
	for _, tc := range []struct {
		path string
		ctyp string
	}{
		{"/public/status", "text/html"},
		{"/public/status.json", "application/json"},
		{"/public/status.rss", "application/rss+xml"},
	} {
		resp, err := anon.Get(ts.URL + tc.path) //nolint:noctx // test, loopback-only
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		gotBody, status := readAndClose(t, resp)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status = %d, body = %s", tc.path, status, gotBody)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, tc.ctyp) {
			t.Errorf("GET %s: Content-Type = %q, want it to contain %q", tc.path, ct, tc.ctyp)
		}
		if !strings.Contains(gotBody, "Checkout API") {
			t.Errorf("GET %s: body does not contain the component name, body = %s", tc.path, gotBody)
		}
		if !strings.Contains(gotBody, "Checkout API degraded") {
			t.Errorf("GET %s: body does not contain the incident title, body = %s", tc.path, gotBody)
		}
	}

	// Step 5: disabling the page through the real authenticated route
	// makes every public route stop serving again, on the very next
	// request, with no separate propagation delay.
	status, body = requestJSON(t, admin, http.MethodPut, ts.URL+"/api/v1/status-page", `{"enabled":false}`)
	if status != http.StatusOK {
		t.Fatalf("disable status page: status = %d, body = %s", status, body)
	}
	for _, path := range []string{"/public/status", "/public/status.json", "/public/status.rss"} {
		if status, body := requestJSON(t, anon, http.MethodGet, ts.URL+path, ""); status != http.StatusNotFound {
			t.Fatalf("GET %s after disabling: status = %d, want %d, body = %s", path, status, http.StatusNotFound, body)
		}
	}
}

// readAndClose reads and closes resp's body: anon.Get above needs the raw
// *http.Response for its Content-Type header, unlike requestJSON's pair.
func readAndClose(t *testing.T, resp *http.Response) (body string, status int) {
	t.Helper()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %v", err)
		}
	}()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return string(b), resp.StatusCode
}
