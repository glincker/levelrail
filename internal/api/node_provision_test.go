package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeNodeProviderSecrets is a hand-written in-memory fake for
// NodeProviderSecrets, the same pattern fakeBackupSecretsSetter already
// establishes, extended with Resolve since this package does call it.
type fakeNodeProviderSecrets struct {
	values map[string]string // serviceName+"/"+envKey -> plaintext
}

func newFakeNodeProviderSecrets() *fakeNodeProviderSecrets {
	return &fakeNodeProviderSecrets{values: map[string]string{}}
}

func (f *fakeNodeProviderSecrets) key(serviceName, envKey string) string {
	return serviceName + "/" + envKey
}

func (f *fakeNodeProviderSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.values[f.key(serviceName, envKey)] = plaintext
	return nil
}

func (f *fakeNodeProviderSecrets) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	_, ok := f.values[f.key(serviceName, envKey)]
	return ok, nil
}

func (f *fakeNodeProviderSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	v, ok := f.values[f.key(serviceName, envKey)]
	if !ok {
		return "", errNodeProviderSecretNotFound
	}
	return v, nil
}

var errNodeProviderSecretNotFound = context.DeadlineExceeded // any distinct sentinel error, unwrapped by callers here

// fakeNodeProvisioner is a hand-written fake for NodeProvisioner.
type fakeNodeProvisioner struct {
	regions []provision.Region
	sizes   []provision.Size

	createServerID string
	createIPAddr   string
	createErr      error

	getStatus provision.ServerStatus
	getIPAddr string
	getErr    error

	deleteErr error

	regionCalls int
	sizeCalls   int
	createCalls int
	getCalls    int
	deletedIDs  []string
}

func (f *fakeNodeProvisioner) ListRegions(context.Context) ([]provision.Region, error) {
	f.regionCalls++
	return f.regions, nil
}

func (f *fakeNodeProvisioner) ListSizes(context.Context, string) ([]provision.Size, error) {
	f.sizeCalls++
	return f.sizes, nil
}

func (f *fakeNodeProvisioner) CreateServer(context.Context, provision.CreateOpts) (string, string, error) {
	f.createCalls++
	return f.createServerID, f.createIPAddr, f.createErr
}

func (f *fakeNodeProvisioner) GetServer(context.Context, string) (provision.ServerStatus, string, error) {
	f.getCalls++
	return f.getStatus, f.getIPAddr, f.getErr
}

func (f *fakeNodeProvisioner) DeleteServer(_ context.Context, id string) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return f.deleteErr
}

func newTestRouterWithNodeProvisioning(t *testing.T, secrets NodeProviderSecrets, fake *fakeNodeProvisioner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	factory := func(string, string) (NodeProvisioner, error) { return fake, nil }
	return NewRouter(logger, testBrand(), db, WithNodeProviderSecrets(secrets), WithNodeProvisionerFactory(factory)), db
}

func TestHandleListNodeProviders(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, &fakeNodeProvisioner{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-providers", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out []nodeProviderResource
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != len(nodeProviderNames) {
		t.Fatalf("providers = %+v, want %d", out, len(nodeProviderNames))
	}
	for _, p := range out {
		if p.Provider == "hetzner" && !p.HasToken {
			t.Errorf("hetzner HasToken = false, want true")
		}
		if p.Provider == "digitalocean" && p.HasToken {
			t.Errorf("digitalocean HasToken = true, want false")
		}
	}
}

func TestHandleSetNodeProviderCredential(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, &fakeNodeProvisioner{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/node-providers", `{"provider":"hetzner","token":"secret"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	exists, _ := secrets.Exists(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey)
	if !exists {
		t.Error("token was not stored")
	}
}

func TestHandleSetNodeProviderCredential_UnknownProvider(t *testing.T) {
	rt, db := newTestRouterWithNodeProvisioning(t, newFakeNodeProviderSecrets(), &fakeNodeProvisioner{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/node-providers", `{"provider":"aws","token":"secret"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleSetNodeProviderCredential_NotConfigured(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/node-providers", `{"provider":"hetzner","token":"secret"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestHandleListNodeProviderRegions_CachesResult(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{regions: []provision.Region{{ID: "fsn1", Name: "Falkenstein"}}}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-providers/hetzner/regions", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}
	if fake.regionCalls != 1 {
		t.Errorf("regionCalls = %d, want 1 (second request should hit the cache)", fake.regionCalls)
	}
}

func TestHandleListNodeProviderSizes(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{sizes: []provision.Size{{ID: "cx22", VCPUs: 2, Memory: 4096, Disk: 40, PriceMonthly: "4.90"}}}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-providers/hetzner/sizes?region=fsn1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out []nodeProviderSizeResource
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].ID != "cx22" || out[0].PriceMonthly != "4.90" {
		t.Errorf("sizes = %+v", out)
	}
}

func TestHandleCreateNodeProvision(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{createServerID: "12345", createIPAddr: "1.2.3.4"}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	body := `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-1","role":"general","control_plane_addr":"cp.example.com:9443"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out nodeProvisionResource
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != store.NodeProvisionStatusBooting {
		t.Errorf("Status = %q, want %q", out.Status, store.NodeProvisionStatusBooting)
	}
	if out.IPAddress != "1.2.3.4" {
		t.Errorf("IPAddress = %q, want 1.2.3.4", out.IPAddress)
	}
	if fake.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", fake.createCalls)
	}

	stored, err := db.GetNodeProvision(context.Background(), out.ID)
	if err != nil {
		t.Fatalf("GetNodeProvision: %v", err)
	}
	if stored.ProviderServerID != "12345" {
		t.Errorf("ProviderServerID = %q, want 12345", stored.ProviderServerID)
	}
}

func TestHandleCreateNodeProvision_ValidationErrors(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, &fakeNodeProvisioner{})
	cookie := loginTestSession(t, rt, db)

	cases := []struct {
		name string
		body string
	}{
		{"unknown provider", `{"provider":"aws","region":"r","size":"s","name":"web-1","control_plane_addr":"cp:9443"}`},
		{"missing region", `{"provider":"hetzner","size":"s","name":"web-1","control_plane_addr":"cp:9443"}`},
		{"missing control plane addr", `{"provider":"hetzner","region":"r","size":"s","name":"web-1"}`},
		{"invalid name", `{"provider":"hetzner","region":"r","size":"s","name":"Web_1","control_plane_addr":"cp:9443"}`},
		{"invalid role", `{"provider":"hetzner","region":"r","size":"s","name":"web-1","role":"admin","control_plane_addr":"cp:9443"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateNodeProvision_ProviderErrorRecordsFailure(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{createErr: &provision.ProviderError{Status: 422, Body: "quota exceeded"}}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	body := `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-1","control_plane_addr":"cp.example.com:9443"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", body))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}

	list, err := db.ListNodeProvisions(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("ListNodeProvisions() = %+v, %v", list, err)
	}
	if list[0].Status != store.NodeProvisionStatusFailed {
		t.Errorf("Status = %q, want failed", list[0].Status)
	}
}

func TestHandleGetNodeProvision_RefreshesToReadyOnceNodeEnrolled(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{getStatus: provision.ServerStatusRunning, getIPAddr: "1.2.3.4"}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	p := store.NodeProvision{
		ID: "npv_1", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-1", Role: "general",
		Status: store.NodeProvisionStatusBooting, ProviderServerID: "999", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveNodeProvision(context.Background(), p); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}
	if err := db.SaveNode(context.Background(), store.Node{ID: "node_1", Name: "web-1", Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-provisions/npv_1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out nodeProvisionResource
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != store.NodeProvisionStatusReady || out.NodeID != "node_1" {
		t.Errorf("out = %+v", out)
	}
}

func TestHandleGetNodeProvision_RunningWithoutEnrollmentReportsEnrolling(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{getStatus: provision.ServerStatusRunning, getIPAddr: "1.2.3.4"}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	p := store.NodeProvision{
		ID: "npv_2", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-2", Role: "general",
		Status: store.NodeProvisionStatusBooting, ProviderServerID: "999", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveNodeProvision(context.Background(), p); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-provisions/npv_2", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out nodeProvisionResource
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != store.NodeProvisionStatusEnrolling {
		t.Errorf("Status = %q, want enrolling", out.Status)
	}
}

func TestHandleGetNodeProvision_TimesOutToFailed(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{getStatus: provision.ServerStatusRunning}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	longAgo := time.Now().Add(-2 * defaultNodeProvisionTimeout)
	p := store.NodeProvision{
		ID: "npv_3", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-3", Role: "general",
		Status: store.NodeProvisionStatusEnrolling, ProviderServerID: "999", CreatedAt: longAgo, UpdatedAt: longAgo,
	}
	if err := db.SaveNodeProvision(context.Background(), p); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-provisions/npv_3", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out nodeProvisionResource
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != store.NodeProvisionStatusFailed || out.FailureReason == "" {
		t.Errorf("out = %+v, want failed with a reason", out)
	}
}

func TestHandleGetNodeProvision_TerminalStatusIsNotRefreshed(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	fake := &fakeNodeProvisioner{}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	p := store.NodeProvision{
		ID: "npv_4", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-4", Role: "general",
		Status: store.NodeProvisionStatusReady, NodeID: "node_x", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveNodeProvision(context.Background(), p); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-provisions/npv_4", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.getCalls != 0 {
		t.Errorf("getCalls = %d, want 0 (a ready provision should not be refreshed)", fake.getCalls)
	}
}

func TestHandleListNodeProvisions_DoesNotRefresh(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	fake := &fakeNodeProvisioner{getStatus: provision.ServerStatusRunning}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	p := store.NodeProvision{
		ID: "npv_5", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-5", Role: "general",
		Status: store.NodeProvisionStatusBooting, ProviderServerID: "999", CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveNodeProvision(context.Background(), p); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-provisions", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.getCalls != 0 {
		t.Errorf("getCalls = %d, want 0 (list must not live-refresh each row)", fake.getCalls)
	}
}

func TestHandleCreateNodeProvision_RejectsNameAlreadyUsedByANode(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, &fakeNodeProvisioner{})
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	if err := db.SaveNode(context.Background(), store.Node{ID: "node_existing", Name: "web-1", Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	body := `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-1","control_plane_addr":"cp.example.com:9443"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateNodeProvision_RejectsNameAlreadyUsedByAnActiveProvision(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, &fakeNodeProvisioner{})
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	if err := db.SaveNodeProvision(context.Background(), store.NodeProvision{
		ID: "npv_existing", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-1", Role: "general",
		Status: store.NodeProvisionStatusBooting, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	body := `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-1","control_plane_addr":"cp.example.com:9443"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateNodeProvision_AllowsNameFromAFailedProvision(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{createServerID: "1", createIPAddr: "1.2.3.4"}
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	cookie := loginTestSession(t, rt, db)

	now := time.Now()
	if err := db.SaveNodeProvision(context.Background(), store.NodeProvision{
		ID: "npv_failed", Provider: "hetzner", Region: "fsn1", Size: "cx22", Name: "web-1", Role: "general",
		Status: store.NodeProvisionStatusFailed, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveNodeProvision: %v", err)
	}

	body := `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-1","control_plane_addr":"cp.example.com:9443"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshNodeProvision_AppliesRoleToTheEnrolledNode(t *testing.T) {
	cases := []struct {
		role      string
		wantApp   bool
		wantBuild bool
	}{
		{role: "general", wantApp: true, wantBuild: false},
		{role: "build", wantApp: false, wantBuild: true},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			secrets := newFakeNodeProviderSecrets()
			_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
			fake := &fakeNodeProvisioner{getStatus: provision.ServerStatusRunning}
			rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
			cookie := loginTestSession(t, rt, db)

			now := time.Now()
			p := store.NodeProvision{
				ID: "npv_role_" + tc.role, Provider: "hetzner", Region: "fsn1", Size: "cx22",
				Name: "web-role-" + tc.role, Role: tc.role,
				Status: store.NodeProvisionStatusEnrolling, ProviderServerID: "999", CreatedAt: now, UpdatedAt: now,
			}
			if err := db.SaveNodeProvision(context.Background(), p); err != nil {
				t.Fatalf("SaveNodeProvision: %v", err)
			}
			if err := db.SaveNode(context.Background(), store.Node{
				ID: "node_" + tc.role, Name: p.Name, Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("SaveNode: %v", err)
			}

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-provisions/"+p.ID, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}

			node, err := db.GetNode(context.Background(), "node_"+tc.role)
			if err != nil {
				t.Fatalf("GetNode: %v", err)
			}
			if node.AcceptsAppWorkloads != tc.wantApp || node.AcceptsBuildWorkloads != tc.wantBuild {
				t.Errorf("role %q: AcceptsAppWorkloads=%t AcceptsBuildWorkloads=%t, want app=%t build=%t",
					tc.role, node.AcceptsAppWorkloads, node.AcceptsBuildWorkloads, tc.wantApp, tc.wantBuild)
			}
		})
	}
}

// failOnceNodeProvisionStore wraps a real NodeProvisionStore, failing the
// Nth call to UpdateNodeProvisionStatus (1-indexed) and delegating every
// other call, for the half-succeeded "CreateServer worked, persisting it
// didn't" path handleCreateNodeProvision must handle.
type failOnceNodeProvisionStore struct {
	NodeProvisionStore
	failOnCall int
	calls      int
}

func (f *failOnceNodeProvisionStore) UpdateNodeProvisionStatus(ctx context.Context, id, status, providerServerID, ipAddress, nodeID, failureReason string, updatedAt time.Time) error {
	f.calls++
	if f.calls == f.failOnCall {
		return fmt.Errorf("simulated write failure")
	}
	return f.NodeProvisionStore.UpdateNodeProvisionStatus(ctx, id, status, providerServerID, ipAddress, nodeID, failureReason, updatedAt)
}

func TestHandleCreateNodeProvision_CleansUpOrphanedServerOnPersistFailure(t *testing.T) {
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeNodeProvisioner{createServerID: "srv-orphan", createIPAddr: "1.2.3.4"}

	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	failingStore := &failOnceNodeProvisionStore{NodeProvisionStore: db, failOnCall: 1}
	factory := func(string, string) (NodeProvisioner, error) { return fake, nil }
	rt := NewRouter(logger, testBrand(), db,
		WithNodeProviderSecrets(secrets),
		WithNodeProvisionerFactory(factory),
		WithNodeProvisions(failingStore),
	)
	cookie := loginTestSession(t, rt, db)

	body := `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"web-1","control_plane_addr":"cp.example.com:9443"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", body))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.deletedIDs) != 1 || fake.deletedIDs[0] != "srv-orphan" {
		t.Errorf("deletedIDs = %v, want [srv-orphan]", fake.deletedIDs)
	}
}
