package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/store"
)

// remoteLister stands in for an agent transport that can list volumes.
type remoteLister struct {
	docker.Runtime
	vols []docker.NamedVolume
}

func (r remoteLister) ListNodeVolumes(context.Context) ([]docker.NamedVolume, error) {
	return r.vols, nil
}

func remoteFixture(t *testing.T, resolve NodeRuntimeResolver) *ownerFixture {
	t.Helper()
	o := newOwnerFixture(t)
	o.f.rt.execRuntime = resolve
	return o
}

func (o *ownerFixture) remoteAllowed(t *testing.T, team, choice string) (bool, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/databases", nil)
	ok := o.f.rt.leftoverChoiceAllowed(rec, r, "ledger", "node-b", team, choice, "")
	return ok, rec
}

func recordOwner(t *testing.T, o *ownerFixture, team string) {
	t.Helper()
	if err := o.f.db.SetDatabaseVolumeOwner(context.Background(), store.DatabaseVolumeOwner{Name: "ledger", TeamID: team}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteLeftoverFailsClosedWhenNodeCannotBeAsked(t *testing.T) {
	t.Parallel()
	offline := func(string) (docker.Runtime, error) { return nil, errors.New("agent offline") }
	oldAgent := func(string) (docker.Runtime, error) { return remoteLifeless{}, nil }
	for name, resolve := range map[string]NodeRuntimeResolver{"offline": offline, "old agent": oldAgent} {
		o := remoteFixture(t, resolve)
		recordOwner(t, o, teamA)
		ok, rec := o.remoteAllowed(t, teamB, "")
		if ok || rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), errCodeLeftoverNodeUnreachable) {
			t.Fatalf("%s: other team = %v %d %s", name, ok, rec.Code, rec.Body.String())
		}
		if ok, _ := o.remoteAllowed(t, teamA, ""); !ok {
			t.Fatalf("%s: the recorded owner must not be blocked", name)
		}
	}
}

// remoteLifeless has no ListNodeVolumes, like an agent from before the op existed.
type remoteLifeless struct{ docker.Runtime }

func TestRemoteLeftoverOwnerChecks(t *testing.T) {
	t.Parallel()
	present := func(string) (docker.Runtime, error) {
		return remoteLister{vols: []docker.NamedVolume{{Name: keptVolume}}}, nil
	}
	o := remoteFixture(t, present)
	recordOwner(t, o, teamA)
	ok, rec := o.remoteAllowed(t, teamB, "")
	if ok || rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), errCodeLeftoverOtherTeam) {
		t.Fatalf("foreign owner = %v %d %s", ok, rec.Code, rec.Body.String())
	}
	if ok, _ := o.remoteAllowed(t, teamA, ""); !ok {
		t.Fatal("true owner refused")
	}
	if ok, _ := o.remoteAllowed(t, teamB, existingVolumeDiscard); !ok {
		t.Fatal("discard must stay possible")
	}

	absent := func(string) (docker.Runtime, error) { return remoteLister{}, nil }
	o = remoteFixture(t, absent)
	recordOwner(t, o, teamA)
	if ok, _ := o.remoteAllowed(t, teamB, ""); !ok {
		t.Fatal("no leftover volume on the node, nothing to refuse")
	}
}

func newOwnerRouter(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	rt, db, _ := newPipelineRouter(t)
	return rt, db
}

func TestAppVolumeReuseVerdict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vols := []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/data"}}
	exists := func(context.Context, string) (bool, bool) { return true, true }
	gone := func(context.Context, string) (bool, bool) { return false, true }
	unknown := func(context.Context, string) (bool, bool) { return false, false }

	t.Run("foreign instance refused, then allowed once the volume is gone", func(t *testing.T) {
		rt, db := newOwnerRouter(t)
		if err := db.SetAppVolumeInstance(ctx, "app-web-data", "old-instance"); err != nil {
			t.Fatal(err)
		}
		if code, _ := rt.appVolumeReuseVerdict(ctx, "web", vols, false, exists); code != errCodeAppVolumeForeign {
			t.Fatalf("code = %q", code)
		}
		if code, _ := rt.appVolumeReuseVerdict(ctx, "web", vols, false, unknown); code != errCodeAppVolumeUnreachable {
			t.Fatalf("unreachable code = %q", code)
		}
		if code, _ := rt.appVolumeReuseVerdict(ctx, "web", vols, false, gone); code != "" {
			t.Fatalf("absent volume code = %q", code)
		}
	})
	t.Run("true owner allowed", func(t *testing.T) {
		rt, db := newOwnerRouter(t)
		id, err := db.ServiceInstanceID(ctx, "web")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.SetAppVolumeInstance(ctx, "app-web-data", id); err != nil {
			t.Fatal(err)
		}
		if code, _ := rt.appVolumeReuseVerdict(ctx, "web", vols, false, exists); code != "" {
			t.Fatalf("code = %q", code)
		}
	})
	t.Run("legacy unlabelled volume of a live app is claimed", func(t *testing.T) {
		rt, db := newOwnerRouter(t)
		if code, _ := rt.appVolumeReuseVerdict(ctx, "web", vols, true, exists); code != "" {
			t.Fatalf("code = %q", code)
		}
		id, _ := db.ServiceInstanceID(ctx, "web")
		if got, _ := db.GetAppVolumeInstance(ctx, "app-web-data"); got != id {
			t.Fatalf("backfilled owner = %q, want %q", got, id)
		}
	})
	t.Run("unrecorded volume under a new app name refused", func(t *testing.T) {
		rt, _ := newOwnerRouter(t)
		if code, _ := rt.appVolumeReuseVerdict(ctx, "web", vols, false, exists); code != errCodeAppVolumeUnknown {
			t.Fatalf("code = %q", code)
		}
	})
}
