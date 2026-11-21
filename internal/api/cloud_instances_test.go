package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudspend"
	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeInstanceProvisioner struct {
	*fakeNodeProvisioner
	actions    []provision.InstanceActionKind
	terminated atomic.Int32
}

func (f *fakeInstanceProvisioner) ListInstances(context.Context, provision.InstanceScope) ([]provision.Instance, error) {
	return []provision.Instance{{ID: "i-1", Name: "web", State: provision.InstanceStateRunning, Managed: true}}, nil
}

func (f *fakeInstanceProvisioner) DescribeInstance(context.Context, string) (provision.InstanceDetail, error) {
	return provision.InstanceDetail{Instance: provision.Instance{ID: "i-1", Managed: true}}, nil
}

func (f *fakeInstanceProvisioner) InstanceAction(_ context.Context, _ string, k provision.InstanceActionKind) error {
	f.actions = append(f.actions, k)
	return nil
}
func (f *fakeInstanceProvisioner) ResizeInstance(context.Context, string, provision.ResizeSpec) error {
	return nil
}
func (f *fakeInstanceProvisioner) SetInstanceTags(context.Context, string, map[string]string) error {
	return nil
}
func (f *fakeInstanceProvisioner) TerminateInstance(context.Context, string, bool) error {
	f.terminated.Add(1)
	return nil
}
func (f *fakeInstanceProvisioner) InstanceEvents(context.Context, string, time.Time) ([]provision.InstanceEvent, error) {
	return nil, nil
}

func cloudInstanceRouter(t *testing.T, fake NodeProvisioner) (*Router, *store.DB) {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("hetzner"), store.NodeProviderTokenEnvKey, "tok")
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithNodeProviderSecrets(secrets),
		WithNodeProvisionerFactory(func(string, string) (NodeProvisioner, error) { return fake, nil }))
	if err := db.CreateCloudAccount(context.Background(), store.CloudAccount{ID: "hetzner-default", Provider: "hetzner", AuthMethod: "static"}); err != nil {
		t.Fatal(err)
	}
	return rt, db
}

func TestCloudInstances_OperateGrantStopsButCannotTerminate(t *testing.T) {
	fake := &fakeInstanceProvisioner{fakeNodeProvisioner: &fakeNodeProvisioner{}}
	rt, db := cloudInstanceRouter(t, fake)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, store.Organization{ID: teamA, Name: teamA, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	u, err := rt.createLocalUserInTeam(ctx, "op@team-a.test", "", teamMatrixPassword, store.TeamRoleAbilities(store.TeamRoleDeveloper), teamA, store.TeamRoleDeveloper)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCloudGrant(ctx, store.CloudGrant{ID: "cg_t1", AccountID: "hetzner-default", ResourceKind: store.CloudResourceAccount,
		PrincipalType: store.CloudPrincipalUser, PrincipalID: u.ID, Actions: []string{store.CloudActionView, store.CloudActionOperate}}); err != nil {
		t.Fatal(err)
	}
	c := withCookie(sessionCookieForTest(t, rt, u.ID))
	base := "/api/v1/cloud-accounts/hetzner-default/instances/i-1"
	if code, body := teamDo(t, rt, http.MethodPost, base+"/actions", `{"action":"stop"}`, c); code != http.StatusAccepted {
		t.Fatalf("operate stop = %d %s", code, body)
	}
	if code, _ := teamDo(t, rt, http.MethodDelete, base, "", c); code == http.StatusNoContent || code == http.StatusOK {
		t.Fatalf("operate holder terminated, code %d", code)
	}
	if fake.terminated.Load() != 0 {
		t.Error("provider terminate was called")
	}
}

type fakeSpendReader struct{ calls atomic.Int32 }

func (f *fakeSpendReader) Provider() cloudspend.Provider { return cloudspend.ProviderHetzner }
func (f *fakeSpendReader) Spend(_ context.Context, q cloudspend.SpendQuery) (cloudspend.SpendReport, error) {
	f.calls.Add(1)
	return cloudspend.SpendReport{AccountID: q.AccountID, State: cloudspend.StateOK}, nil
}
func (f *fakeSpendReader) Forecast(context.Context, cloudspend.SpendQuery) (cloudspend.SpendForecast, error) {
	return cloudspend.SpendForecast{}, cloudspend.ErrUnsupported
}
func (f *fakeSpendReader) Budgets(context.Context, cloudspend.SpendQuery) ([]cloudspend.ProviderBudget, error) {
	return nil, nil
}

func TestCloudSpend_RefreshTwiceIsRateLimited(t *testing.T) {
	old := newCloudSpendReader
	fake := &fakeSpendReader{}
	newCloudSpendReader = func(cloudspend.Provider, cloudspend.Credentials, func() cloudspend.Config) (cloudspend.Reader, error) {
		return fake, nil
	}
	t.Cleanup(func() { newCloudSpendReader = old })
	rt, db := cloudInstanceRouter(t, &fakeNodeProvisioner{})
	cookie := withCookie(loginTestSession(t, rt, db))
	url := "/api/v1/cloud-accounts/hetzner-default/spend/refresh"
	if code, body := teamDo(t, rt, http.MethodPost, url, "", cookie); code != http.StatusOK {
		t.Fatalf("first refresh = %d %s", code, body)
	}
	if code, _ := teamDo(t, rt, http.MethodPost, url, "", cookie); code != http.StatusTooManyRequests {
		t.Fatalf("second refresh = %d, want 429", code)
	}
}

func TestMeCloudAccess_GrantHolderSeesOnlyHeldActions(t *testing.T) {
	rt, db := cloudInstanceRouter(t, &fakeInstanceProvisioner{fakeNodeProvisioner: &fakeNodeProvisioner{}})
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, store.Organization{ID: teamA, Name: teamA, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	u, err := rt.createLocalUserInTeam(ctx, "ca@team-a.test", "", teamMatrixPassword, store.TeamRoleAbilities(store.TeamRoleDeveloper), teamA, store.TeamRoleDeveloper)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCloudGrant(ctx, store.CloudGrant{ID: "cg_t2", AccountID: "hetzner-default", ResourceKind: store.CloudResourceAccount,
		PrincipalType: store.CloudPrincipalUser, PrincipalID: u.ID, Actions: []string{store.CloudActionView}}); err != nil {
		t.Fatal(err)
	}
	code, body := teamDo(t, rt, http.MethodGet, "/api/v1/me/cloud-access", "", withCookie(sessionCookieForTest(t, rt, u.ID)))
	if code != http.StatusOK || !strings.Contains(body, `"hetzner-default":["view"]`) {
		t.Fatalf("me/cloud-access = %d %s", code, body)
	}
}
