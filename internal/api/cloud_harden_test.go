package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/cloudspend"
	"github.com/glincker/levelrail/internal/store"
)

func TestAuthorizeCloudGrantExpiryBound(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	soon, later, past := now.Add(time.Hour), now.Add(2*time.Hour), now.Add(-time.Minute)
	comp := cloudCompartmentResource("oracle-prod", "comp1")
	held := func(exp *time.Time) []store.CloudGrant {
		return []store.CloudGrant{cloudTestGrant(store.CloudResourceAccount, "", exp, store.CloudActionShare, store.CloudActionView)}
	}
	tests := []struct {
		name    string
		in      cloudGrantRequest
		wantErr bool
	}{
		{"later than grantor refused", cloudGrantRequest{ExpiresAt: &later, GrantorGrants: held(&soon)}, true},
		{"equal to grantor passes", cloudGrantRequest{ExpiresAt: &soon, GrantorGrants: held(&soon)}, false},
		{"earlier passes", cloudGrantRequest{ExpiresAt: &now, GrantorGrants: held(&soon)}, false},
		{"latest live grant bounds", cloudGrantRequest{ExpiresAt: &later, GrantorGrants: append(held(&soon), held(&later)...)}, false},
		{"expired grant is ignored", cloudGrantRequest{ExpiresAt: &later, GrantorGrants: append(held(&past), held(&soon)...)}, true},
		{"unbounded grantor passes any", cloudGrantRequest{ExpiresAt: &later, GrantorGrants: held(nil)}, false},
		{"root unbounded", cloudGrantRequest{Root: true, GrantorGrants: held(&soon)}, false},
		{"owner team admin unbounded", cloudGrantRequest{OwnerTeamAdmin: true, GrantorGrants: held(&soon)}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Now, tc.in.Resource, tc.in.Actions = now, comp, []string{store.CloudActionView}
			if err := authorizeCloudGrant(tc.in); (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestLinkKitRefusesZoneTiersWithoutZones(t *testing.T) {
	tests := []struct {
		name    string
		caps    []string
		zones   []string
		wantErr bool
	}{
		{"dns without zones", []string{"dns"}, nil, true},
		{"destructive without zones", []string{"inventory", "destructive"}, nil, true},
		{"all tiers without zones", nil, nil, true},
		{"dns with zones", []string{"dns"}, []string{"Z123"}, false},
		{"inventory only", []string{"inventory"}, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := requireZonesForZoneTiers(tc.caps, tc.zones); (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

type countingSpendReader struct {
	fakeSpendReader
	forecasts, budgets int32
	mu                 sync.Mutex
}

func (c *countingSpendReader) Forecast(context.Context, cloudspend.SpendQuery) (cloudspend.SpendForecast, error) {
	c.mu.Lock()
	c.forecasts++
	c.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	return cloudspend.SpendForecast{}, nil
}

func (c *countingSpendReader) Budgets(context.Context, cloudspend.SpendQuery) ([]cloudspend.ProviderBudget, error) {
	c.mu.Lock()
	c.budgets++
	c.mu.Unlock()
	return nil, nil
}

func TestSpendWireConcurrentCallsMakeOneCallSet(t *testing.T) {
	rt, _ := cloudInstanceRouter(t, &fakeNodeProvisioner{})
	rd := &countingSpendReader{}
	a := store.CloudAccount{ID: "hetzner-default", Provider: "hetzner"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rt.spendWire(context.Background(), a, rd, cloudspend.SpendReport{}, false)
		}()
	}
	wg.Wait()
	if rd.forecasts != 1 || rd.budgets != 1 {
		t.Errorf("paid calls = %d forecasts, %d budgets, want 1 each", rd.forecasts, rd.budgets)
	}
}

func TestDropCloudGrantsForRemovedPrincipal(t *testing.T) {
	rt, db := cloudInstanceRouter(t, &fakeNodeProvisioner{})
	ctx := context.Background()
	for i, p := range []string{"u1", "u2"} {
		g := store.CloudGrant{ID: "cg_d" + p, AccountID: "hetzner-default", ResourceKind: store.CloudResourceAccount, PrincipalType: store.CloudPrincipalUser, PrincipalID: p, Actions: []string{store.CloudActionView}}
		if err := db.UpsertCloudGrant(ctx, g); err != nil {
			t.Fatal(i, err)
		}
	}
	rt.dropCloudGrantsFor(ctx, store.CloudPrincipalUser, "u1")
	left, err := db.ListCloudGrants(ctx, "hetzner-default")
	if err != nil || len(left) != 1 || left[0].PrincipalID != "u2" {
		t.Fatalf("grants after drop = %+v, %v", left, err)
	}
}

func TestCloudScopedRootTokenStaysInItsTeam(t *testing.T) {
	rt, db := cloudInstanceRouter(t, &fakeNodeProvisioner{})
	ctx := context.Background()
	for _, id := range []string{teamA, teamB} {
		if err := db.SaveOrganization(ctx, store.Organization{ID: id, Name: id, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if err := db.CreateCloudAccount(ctx, store.CloudAccount{ID: "acct-" + id, Provider: "hetzner", AuthMethod: "static", TeamID: id}); err != nil {
			t.Fatal(err)
		}
	}
	plain := "scoped-root-secret"
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "tok-root-a", Name: "root-a", TokenHash: hashToken(plain), Abilities: []string{AbilityRoot, AbilityRead}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAPITokenScope(ctx, store.APITokenScope{TokenID: "tok-root-a", TeamID: teamA, Role: store.TeamRoleOwner}); err != nil {
		t.Fatal(err)
	}
	if code, body := teamDo(t, rt, http.MethodGet, "/api/v1/cloud-accounts/acct-"+teamA, "", bearer(plain)); code != http.StatusOK {
		t.Errorf("own team account = %d %s", code, body)
	}
	if code, _ := teamDo(t, rt, http.MethodGet, "/api/v1/cloud-accounts/acct-"+teamB, "", bearer(plain)); code != http.StatusNotFound && code != http.StatusForbidden {
		t.Errorf("other team account = %d, want 403 or 404", code)
	}
}

type flakyForecastReader struct {
	fakeSpendReader
	fail bool
}

func (f *flakyForecastReader) Forecast(context.Context, cloudspend.SpendQuery) (cloudspend.SpendForecast, error) {
	if f.fail {
		return cloudspend.SpendForecast{}, errors.New("throttled")
	}
	return cloudspend.SpendForecast{Amount: 42, Currency: "USD"}, nil
}

func TestSpendWireKeepsLastGoodForecastWhenRefreshFails(t *testing.T) {
	rt, _ := cloudInstanceRouter(t, &fakeNodeProvisioner{})
	rd := &flakyForecastReader{}
	a := store.CloudAccount{ID: "oracle-keep", Provider: "oracle"}
	if w := rt.spendWire(context.Background(), a, rd, cloudspend.SpendReport{}, true); w.Forecast == nil {
		t.Fatal("first read must have a forecast")
	}
	rd.fail = true
	w := rt.spendWire(context.Background(), a, rd, cloudspend.SpendReport{}, true)
	if w.Forecast == nil || w.Forecast.Amount != 42 {
		t.Fatalf("forecast after failed refresh = %+v", w.Forecast)
	}
}
