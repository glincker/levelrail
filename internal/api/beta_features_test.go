package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func TestBetaFeatures_ListDescribesEveryGatedFeature(t *testing.T) {
	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/beta-features", ""))
	var list []betaFeatureResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != len(experimental.All()) {
		t.Fatalf("listed %d features, want %d", len(list), len(experimental.All()))
	}
	byID := map[string]betaFeatureResource{}
	for _, f := range list {
		byID[f.ID] = f
	}
	cases := []struct {
		id         string
		toggleable bool
		restart    bool
		maturity   string
	}{
		{"load-balancer", true, true, experimental.MaturityExperimental},
		{"iac", true, true, experimental.MaturityExperimental},
		{"resource-advisor", true, false, experimental.MaturityBeta},
		{"ai-chat", true, false, experimental.MaturityBeta},
		{"canvas", true, false, experimental.MaturityBeta},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			f, ok := byID[c.id]
			if !ok {
				t.Fatalf("%s missing from list", c.id)
			}
			if f.Toggleable != c.toggleable || f.RequiresRestart != c.restart || f.Maturity != c.maturity {
				t.Fatalf("got toggleable=%v restart=%v maturity=%s", f.Toggleable, f.RequiresRestart, f.Maturity)
			}
			if f.Name == "" || f.Summary == "" || f.DocsPath == "" || f.EnabledBy != betaSourceOff {
				t.Fatalf("incomplete description: %+v", f)
			}
			if !strings.Contains(f.EnvHint, "APP_EXPERIMENTAL="+c.id) || !strings.Contains(f.EnvHint, "systemd") || !strings.Contains(f.EnvHint, "docker") {
				t.Fatalf("env hint = %q", f.EnvHint)
			}
		})
	}
}

func TestBetaFeatures_PutRejections(t *testing.T) {
	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	cases := []struct {
		name, feature string
		want          int
		contains      string
	}{
		{"unknown feature", "nope", http.StatusNotFound, "no gated feature"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/beta-features/"+c.feature, `{"enabled":true}`))
			if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.contains) {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
			if experimental.Enabled(experimental.Feature(c.feature)) {
				t.Fatalf("%s switched on despite rejection", c.feature)
			}
		})
	}
}

func TestBetaFeatures_StartupWiredSavesAndFlagsRestart(t *testing.T) {
	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/beta-features/load-balancer", `{"enabled":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var res betaFeatureResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Enabled || !res.PendingRestart || res.AppliesLive || res.Locked {
		t.Fatalf("want saved but pending restart: %+v", res)
	}
	stored, err := db.ListBetaFeatures(context.Background())
	if err != nil || !stored["load-balancer"] {
		t.Fatalf("not persisted: %v %v", stored, err)
	}
	experimental.Set()
	if err := LoadBetaFeatures(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if !experimental.Enabled(experimental.LoadBalancer) || experimental.Pending(experimental.LoadBalancer) {
		t.Fatal("boot must apply the saved switch and clear the restart flag")
	}
}

func TestBetaFeatures_EnvOverrideIsLocked(t *testing.T) {
	experimental.Set(experimental.IaC)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	res := betaFeatureState(experimental.IaC)
	if !res.Enabled || !res.Locked || res.EnabledBy != betaSourceEnv || res.PendingRestart {
		t.Fatalf("env feature must be locked and not pending: %+v", res)
	}
}
