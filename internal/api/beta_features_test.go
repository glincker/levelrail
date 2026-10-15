package api

import (
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
		{"load-balancer", false, true, experimental.MaturityExperimental},
		{"iac", false, true, experimental.MaturityExperimental},
		{"resource-advisor", true, false, experimental.MaturityBeta},
		{"ai-chat", true, false, experimental.MaturityBeta},
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
		{"env only feature", "load-balancer", http.StatusConflict, "APP_EXPERIMENTAL=load-balancer"},
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
