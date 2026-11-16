package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func guardFixture(t *testing.T, fake *fakeNodeProvisioner) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	rt, db := newTestRouterWithNodeProvisioning(t, secrets, fake)
	return rt, db, loginTestSession(t, rt, db)
}

func saveProv(t *testing.T, db *store.DB, p store.NodeProvision) {
	t.Helper()
	if p.Provider == "" {
		p.Provider = "hetzner"
	}
	if p.Role == "" {
		p.Role = "general"
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if err := db.SaveNodeProvision(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func provReq(rt *Router, cookie *http.Cookie, t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func createBody(name string) string {
	return `{"provider":"hetzner","region":"fsn1","size":"cx22","name":"` + name + `","control_plane_addr":"cp.example.com:9443"}`
}

func setSetting(t *testing.T, rt *Router, key, raw string) {
	t.Helper()
	if _, err := rt.runtimeSettings.Set(context.Background(), key, json.RawMessage(raw), "test"); err != nil {
		t.Fatal(err)
	}
}

func TestProvisionCountsAgainstCap_FailedWithServer(t *testing.T) {
	leaked := store.NodeProvision{Status: store.NodeProvisionStatusFailed, ProviderServerID: "s1", FailureReason: "errored"}
	if !provisionCountsAgainstCap(leaked) {
		t.Fatal("failed row with an undeleted server must count")
	}
	leaked.FailureReason = "errored" + provisionServerDeletedNote
	if provisionCountsAgainstCap(leaked) {
		t.Fatal("failed row with a deleted server must not count")
	}
	if provisionCountsAgainstCap(store.NodeProvision{Status: store.NodeProvisionStatusFailed}) {
		t.Fatal("failed row with no server must not count")
	}
}

func TestRefreshNodeProvision_ErroredServerIsDeleted(t *testing.T) {
	fake := &fakeNodeProvisioner{getStatus: provision.ServerStatusError}
	rt, db, cookie := guardFixture(t, fake)
	saveProv(t, db, store.NodeProvision{ID: "npv_e1", Name: "web-e1", Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-e1"})

	rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/node-provisions/npv_e1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(fake.deletedIDs) != 1 || fake.deletedIDs[0] != "srv-e1" {
		t.Fatalf("deletedIDs = %v", fake.deletedIDs)
	}
	got, _ := db.GetNodeProvision(context.Background(), "npv_e1")
	if got.Status != store.NodeProvisionStatusFailed || got.ProviderServerID != "srv-e1" || provisionCountsAgainstCap(got) {
		t.Fatalf("row = %+v", got)
	}
}

func TestRefreshNodeProvision_ErroredServerDeleteFailureKeepsCounting(t *testing.T) {
	fake := &fakeNodeProvisioner{getStatus: provision.ServerStatusError, deleteErr: &provision.ProviderError{Status: 500, Body: "secret-body"}}
	rt, db, cookie := guardFixture(t, fake)
	saveProv(t, db, store.NodeProvision{ID: "npv_e2", Name: "web-e2", Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-e2"})

	rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/node-provisions/npv_e2", "")
	if strings.Contains(rec.Body.String(), "secret-body") {
		t.Fatalf("provider body leaked: %s", rec.Body.String())
	}
	got, _ := db.GetNodeProvision(context.Background(), "npv_e2")
	if !provisionCountsAgainstCap(got) {
		t.Fatalf("undeleted errored server must still count: %+v", got)
	}
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody("web-e3")); rec.Code != http.StatusConflict {
		t.Fatalf("create at cap status = %d, want 409", rec.Code)
	}
}

func TestCreate_ClientDisconnectStillRecordsServerID(t *testing.T) {
	fake := &fakeNodeProvisioner{createServerID: "srv-dc", createIPAddr: "1.1.1.1"}
	rt, db, cookie := guardFixture(t, fake)
	ctx, cancel := context.WithCancel(context.Background())
	hook := &cancelOnCreate{fakeNodeProvisioner: fake, cancel: cancel}
	rt.nodeProvisionerFactory = func(string, string) (NodeProvisioner, error) { return hook, nil }

	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", createBody("web-dc")).WithContext(ctx)
	rt.Handler().ServeHTTP(httptest.NewRecorder(), req)

	if hook.ctxErr != nil {
		t.Fatalf("CreateServer ctx was cancelled with the caller: %v", hook.ctxErr)
	}
	rows, _ := db.ListNodeProvisions(context.Background())
	if len(rows) != 1 || rows[0].ProviderServerID != "srv-dc" || rows[0].Status != store.NodeProvisionStatusBooting {
		t.Fatalf("rows = %+v, want one booting row holding the server id", rows)
	}
}

type cancelOnCreate struct {
	*fakeNodeProvisioner
	cancel context.CancelFunc
	ctxErr error
}

func (c *cancelOnCreate) CreateServer(ctx context.Context, opts provision.CreateOpts) (string, string, error) {
	c.cancel()
	c.ctxErr = ctx.Err()
	return c.fakeNodeProvisioner.CreateServer(ctx, opts)
}

func TestCreate_ProviderErrorIsRedacted(t *testing.T) {
	fake := &fakeNodeProvisioner{createErr: &provision.ProviderError{Status: 422, Body: "secret-body"}}
	rt, db, cookie := guardFixture(t, fake)
	rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody("web-r1"))
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "secret-body") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	rows, _ := db.ListNodeProvisions(context.Background())
	if len(rows) != 1 || strings.Contains(rows[0].FailureReason, "secret-body") || !strings.Contains(rows[0].FailureReason, "422") {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDestroy_CreatingWithoutServerIs409(t *testing.T) {
	fake := &fakeNodeProvisioner{}
	rt, db, cookie := guardFixture(t, fake)
	saveProv(t, db, store.NodeProvision{ID: "npv_c1", Name: "web-c1", Status: store.NodeProvisionStatusCreating})
	if rec := provReq(rt, cookie, t, http.MethodDelete, "/api/v1/node-provisions/npv_c1", ""); rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	got, _ := db.GetNodeProvision(context.Background(), "npv_c1")
	if got.Status != store.NodeProvisionStatusCreating {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestStaleRefreshCannotOverwriteDestroyed(t *testing.T) {
	fake := &fakeNodeProvisioner{}
	rt, db, _ := guardFixture(t, fake)
	stale := store.NodeProvision{ID: "npv_s1", Name: "web-s1", Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-s1"}
	saveProv(t, db, stale)
	stale, _ = db.GetNodeProvision(context.Background(), "npv_s1")
	if err := db.UpdateNodeProvisionStatus(context.Background(), "npv_s1", store.NodeProvisionStatusDestroyed, "", "", "", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	out := rt.saveNodeProvisionUpdate(context.Background(), stale, store.NodeProvisionStatusEnrolling, "1.2.3.4", "", "")
	got, _ := db.GetNodeProvision(context.Background(), "npv_s1")
	if got.Status != store.NodeProvisionStatusDestroyed || out.Status != store.NodeProvisionStatusDestroyed {
		t.Fatalf("stored %q returned %q, want destroyed", got.Status, out.Status)
	}
}

func TestDestroy_WorkloadsForceAndRetry(t *testing.T) {
	fake := &fakeNodeProvisioner{}
	rt, db, cookie := guardFixture(t, fake)
	ctx := context.Background()
	now := time.Now()
	if err := db.SaveNode(ctx, store.Node{ID: "node_w", Name: "web-w", Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "svc", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceNode(ctx, "svc", "node_w"); err != nil {
		t.Fatal(err)
	}
	saveProv(t, db, store.NodeProvision{ID: "npv_w", Name: "web-w", Status: store.NodeProvisionStatusReady, ProviderServerID: "srv-w", NodeID: "node_w"})

	if rec := provReq(rt, cookie, t, http.MethodDelete, "/api/v1/node-provisions/npv_w", ""); rec.Code != http.StatusConflict {
		t.Fatalf("busy node status = %d, want 409", rec.Code)
	}
	if len(fake.deletedIDs) != 0 {
		t.Fatalf("deleted despite workloads: %v", fake.deletedIDs)
	}
	// Failed delete leaves the row Destroying, and a retry from there succeeds.
	fake.deleteErr = &provision.ProviderError{Status: 500, Body: "boom"}
	if rec := provReq(rt, cookie, t, http.MethodDelete, "/api/v1/node-provisions/npv_w?force=true", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("force with provider failure status = %d, want 502", rec.Code)
	}
	if got, _ := db.GetNodeProvision(ctx, "npv_w"); got.Status != store.NodeProvisionStatusDestroying {
		t.Fatalf("status = %q, want destroying", got.Status)
	}
	// A provider 404 means it is already gone: idempotent success.
	fake.deleteErr = &provision.ProviderError{Status: http.StatusNotFound, Body: "gone"}
	if rec := provReq(rt, cookie, t, http.MethodDelete, "/api/v1/node-provisions/npv_w?force=true", ""); rec.Code != http.StatusOK {
		t.Fatalf("retry from destroying status = %d, want 200", rec.Code)
	}
	if got, _ := db.GetNodeProvision(ctx, "npv_w"); got.Status != store.NodeProvisionStatusDestroyed {
		t.Fatalf("status = %q, want destroyed", got.Status)
	}
}

func TestPriceCap_EURConversionAndUnknownPrice(t *testing.T) {
	fake := &fakeNodeProvisioner{sizes: []provision.Size{{ID: "cx22", PriceMonthly: "10.00", Currency: "EUR"}}}
	rt, db, cookie := guardFixture(t, fake)
	_ = db
	setSetting(t, rt, runtimesettings.KeyProvisionMaxMonthlyPrice, `"12"`)
	setSetting(t, rt, runtimesettings.KeyProvisionEURToUSDRate, `"1.5"`)
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody("web-p1")); rec.Code != http.StatusConflict {
		t.Fatalf("15 USD over a 12 cap status = %d, want 409", rec.Code)
	}
	setSetting(t, rt, runtimesettings.KeyProvisionEURToUSDRate, `"1.1"`)
	fake.createServerID = "srv-p"
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody("web-p2")); rec.Code != http.StatusCreated {
		t.Fatalf("11 USD under a 12 cap status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPriceCap_UnknownPriceNeedsOptIn(t *testing.T) {
	fake := &fakeNodeProvisioner{createServerID: "srv-u", sizes: []provision.Size{{ID: "cx22", PriceMonthly: "", Currency: ""}}}
	rt, _, cookie := guardFixture(t, fake)
	setSetting(t, rt, runtimesettings.KeyProvisionMaxMonthlyPrice, `"12"`)
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody("web-u1")); rec.Code != http.StatusConflict {
		t.Fatalf("unpriced status = %d, want 409", rec.Code)
	}
	setSetting(t, rt, runtimesettings.KeyProvisionAllowUnknownPrice, `true`)
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody("web-u2")); rec.Code != http.StatusCreated {
		t.Fatalf("opted-in status = %d", rec.Code)
	}
}

func TestListedMonthlyUSD_FailsClosedOnBadNumbers(t *testing.T) {
	for _, price := range []string{"NaN", "Inf", "-1", "-Inf"} {
		if _, ok := listedMonthlyUSD([]provision.Size{{ID: "a", PriceMonthly: price, Currency: "USD"}}, "a", 1.1); ok {
			t.Errorf("price %q accepted", price)
		}
	}
	for _, rate := range []float64{math.NaN(), math.Inf(1), -1, 0} {
		if _, ok := listedMonthlyUSD([]provision.Size{{ID: "a", PriceMonthly: "5", Currency: "EUR"}}, "a", rate); ok {
			t.Errorf("rate %v accepted", rate)
		}
	}
	if usd, ok := listedMonthlyUSD([]provision.Size{{ID: "a", PriceMonthly: "10", Currency: "EUR"}}, "a", 1.5); !ok || usd != 15 {
		t.Errorf("usd = %v ok = %v", usd, ok)
	}
}

func TestEURToUSD_BadRateFailsClosed(t *testing.T) {
	rt, _, _ := guardFixture(t, &fakeNodeProvisioner{})
	if got := rt.eurToUSD(context.Background()); got != 1.10 {
		t.Fatalf("default rate = %v", got)
	}
	t.Setenv("APP_PROVISION_EUR_TO_USD_RATE", "NaN")
	rt.runtimeSettings = nil
	if got := rt.eurToUSD(context.Background()); got != 0 {
		t.Fatalf("NaN rate = %v, want 0", got)
	}
}

func TestCreate_ConcurrentSameNameOneWins(t *testing.T) {
	fake := &fakeNodeProvisioner{createServerID: "srv-cc"}
	rt, _, cookie := guardFixture(t, fake)
	setSetting(t, rt, runtimesettings.KeyProvisionMaxServers, `5`)
	var created int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody("web-cc")); rec.Code == http.StatusCreated {
				atomic.AddInt32(&created, 1)
			}
		}()
	}
	wg.Wait()
	if created != 1 {
		t.Fatalf("created = %d, want exactly 1", created)
	}
}

func TestCreateAndDestroy_ConcurrentCapHolds(t *testing.T) {
	fake := &fakeNodeProvisioner{createServerID: "srv-cd"}
	rt, db, cookie := guardFixture(t, fake)
	saveProv(t, db, store.NodeProvision{ID: "npv_cd", Name: "web-old", Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-old"})
	setSetting(t, rt, runtimesettings.KeyProvisionMaxServers, `2`)
	var created int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			name := "web-n" + strconv.Itoa(i)
			if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/nodes/provision", createBody(name)); rec.Code == http.StatusCreated {
				atomic.AddInt32(&created, 1)
			}
		}(i)
		go func() {
			defer wg.Done()
			provReq(rt, cookie, t, http.MethodDelete, "/api/v1/node-provisions/npv_cd", "")
		}()
	}
	wg.Wait()
	rows, _ := db.ListNodeProvisions(context.Background())
	live := 0
	for _, p := range rows {
		if provisionCountsAgainstCap(p) {
			live++
		}
	}
	if live > 2 {
		t.Fatalf("live = %d exceeds cap 2", live)
	}
	if got, _ := db.GetNodeProvision(context.Background(), "npv_cd"); got.Status != store.NodeProvisionStatusDestroyed {
		t.Fatalf("old row = %q, want destroyed", got.Status)
	}
}

func TestList_ReconcilesStaleRows(t *testing.T) {
	fake := &fakeNodeProvisioner{}
	rt, db, cookie := guardFixture(t, fake)
	old := time.Now().Add(-48 * time.Hour)
	saveProv(t, db, store.NodeProvision{ID: "npv_r1", Name: "web-r1", Status: store.NodeProvisionStatusCreating, CreatedAt: old})
	saveProv(t, db, store.NodeProvision{ID: "npv_r2", Name: "web-r2", Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-r2", CreatedAt: old})
	if rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/node-provisions", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	r1, _ := db.GetNodeProvision(context.Background(), "npv_r1")
	r2, _ := db.GetNodeProvision(context.Background(), "npv_r2")
	if r1.Status != store.NodeProvisionStatusFailed || r2.Status != store.NodeProvisionStatusFailed {
		t.Fatalf("r1 = %q r2 = %q, want both failed", r1.Status, r2.Status)
	}
	if len(fake.deletedIDs) != 1 || fake.deletedIDs[0] != "srv-r2" || provisionCountsAgainstCap(r2) {
		t.Fatalf("deleted = %v r2 = %+v", fake.deletedIDs, r2)
	}
}

func TestOracleSettingsReadLive(t *testing.T) {
	fake := &fakeNodeProvisioner{}
	rt, _, _ := guardFixture(t, fake)
	if rt.oracleBool(context.Background(), runtimesettings.KeyProvisionOracleEnabled, "APP_PROVISION_ORACLE_ENABLED") {
		t.Fatal("oracle must default off")
	}
	if n := rt.oracleLimit(context.Background(), runtimesettings.KeyProvisionOracleMaxA1OCPU, "APP_PROVISION_ORACLE_MAX_A1_OCPU"); n != 0 {
		t.Fatalf("a1 limit default = %d, want 0", n)
	}
	setSetting(t, rt, runtimesettings.KeyProvisionOracleAvailDomains, `"AD-1, AD-2"`)
	setSetting(t, rt, runtimesettings.KeyProvisionOracleBootVolumeGB, `60`)
	s := rt.oracleSettings(context.Background())
	if len(s.AvailabilityDomains) != 2 || s.AvailabilityDomains[1] != "AD-2" || s.BootVolumeGB != 60 || s.FlexPresets != "1:6,2:12" {
		t.Fatalf("settings = %+v", s)
	}
}
