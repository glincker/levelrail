package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
)

type fakeNetAdmin struct {
	details []docker.NetworkDetail
	usage   docker.PoolUsage
	removed []string
}

func (f *fakeNetAdmin) Ping(context.Context) error { return nil }

func (f *fakeNetAdmin) DescribeNetworks(context.Context, string) ([]docker.NetworkDetail, error) {
	return f.details, nil
}

func (f *fakeNetAdmin) RemoveNetworkIfEmpty(_ context.Context, name string) (bool, error) {
	f.removed = append(f.removed, name)
	return true, nil
}

func (f *fakeNetAdmin) NetworkPoolUsage(context.Context) (docker.PoolUsage, error) {
	return f.usage, nil
}

func TestSystemNetworksListAndPrune(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	prefix := application.NetworkPrefixOrDefault(rt.brand.ShortName)
	old := time.Now().Add(-time.Hour)
	instanceID, err := db.GetOrCreateInstanceID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	own := map[string]string{appspec.InstanceLabelKey: instanceID}
	fake := &fakeNetAdmin{
		usage: docker.PoolUsage{Pool: "10.88.0.0/14", Enabled: true, Capacity: 100, InUse: 90},
		details: []docker.NetworkDetail{
			{Name: prefix + "-app-gone", Labels: own, Created: old, Subnets: []string{"10.88.0.0/24"}},
			{Name: prefix + "-app-busy", Labels: own, Created: old, Attached: 1},
		},
	}
	rt.dockerPinger = fake

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/networks", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	var res systemNetworksResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Networks) != 2 || res.Pool.Percent != 90 {
		t.Fatalf("response %+v", res)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/networks/prune", `{"dry_run":true}`))
	if rec.Code != http.StatusOK || len(fake.removed) != 0 {
		t.Fatalf("dry run = %d removed %v", rec.Code, fake.removed)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/networks/prune", `{}`))
	if rec.Code != http.StatusOK || len(fake.removed) != 1 || fake.removed[0] != prefix+"-app-gone" {
		t.Fatalf("prune = %d removed %v", rec.Code, fake.removed)
	}

	if c := rt.doctorCheckNetworkPool(context.Background()); c.Status != doctorStatusWarn {
		t.Fatalf("doctor at 90%% = %+v", c)
	}
	fake.usage.InUse = 10
	if c := rt.doctorCheckNetworkPool(context.Background()); c.Status != doctorStatusOK {
		t.Fatalf("doctor at 10%% = %+v", c)
	}
}
