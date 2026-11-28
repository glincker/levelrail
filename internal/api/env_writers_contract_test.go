package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/envrules"
	"github.com/glincker/levelrail/internal/envrules/contract"
	"github.com/glincker/levelrail/internal/store"
)

// Each writer below must refuse and accept exactly what the shared fixture says.
func TestEnvWritersFollowSharedFixture(t *testing.T) {
	rt := &Router{}
	ctx := context.Background()
	plain := func(c contract.Case) bool {
		// JSON bodies keep a trailing CR byte for byte, so the crlf case is not theirs.
		for _, w := range c.WantErrors {
			if w == "crlf" {
				return false
			}
		}
		return c.DefaultPolicyOnly() && !c.Options.Secret && len(c.Existing.Names) == 0 && len(c.Existing.Plain) == 0 && !c.Options.ExistingUnknown && !c.Options.Multiline
	}
	t.Run("template install env", func(t *testing.T) {
		contract.Run(t, 20, func(c contract.Case) (bool, bool) {
			if !plain(c) {
				return false, true
			}
			return rt.templateEnvRefusal(ctx, c.Set) != "", false
		})
	})
	t.Run("bundle import env", func(t *testing.T) {
		contract.Run(t, 20, func(c contract.Case) (bool, bool) {
			if !plain(c) {
				return false, true
			}
			return rt.bundleEnvRefusal(ctx, c.Set, nil) != "", false
		})
	})
	t.Run("bulk set env", func(t *testing.T) {
		contract.Run(t, 20, func(c contract.Case) (bool, bool) {
			if !plain(c) || len(c.Set) != 1 {
				return false, true
			}
			for k, v := range c.Set {
				return rt.bulkEnvRefusal(ctx, k, v, &store.DesiredService{}) != "", false
			}
			return false, true
		})
	})
}

func TestBuildReportNamesEveryRejectedKey(t *testing.T) {
	set := map[string]string{"GOOD": "1", "1BAD": "x", "LEVELRAIL_X": "y"}
	rep := envrules.BuildReport(set, envrules.Validate(set, envrules.Existing{}, envrules.DefaultPolicy(), envrules.Options{Multiline: true}))
	if len(rep.Accepted) != 1 || rep.Accepted[0] != "GOOD" || len(rep.Rejected) != 2 {
		t.Fatalf("report = %+v", rep)
	}
}

// Preview, branch and PR overrides go through the real handlers and must give the same
// verdict as PUT env for every fixture case that fits one key.
func TestOverrideWritersMatchPutEnv(t *testing.T) {
	type surface struct {
		name   string
		secret bool
		call   func(key, value string, secret bool) (method, path, body string)
	}
	surfaces := []surface{
		{"put env", false, func(k, v string, _ bool) (string, string, string) {
			b, _ := json.Marshal(map[string]map[string]string{"set": {k: v}})
			return http.MethodPut, "/api/v1/apps/web/env", string(b)
		}},
		{"preview override", false, func(k, v string, _ bool) (string, string, string) {
			b, _ := json.Marshal(map[string]string{"value": v})
			return http.MethodPut, "/api/v1/apps/web/preview-env/" + url.PathEscape(k), string(b)
		}},
		{"branch override", true, func(k, v string, s bool) (string, string, string) {
			b, _ := json.Marshal(map[string]any{"branch_pattern": "release/*", "key": k, "value": v, "secret": s})
			return http.MethodPost, "/api/v1/apps/web/branch-env", string(b)
		}},
		{"pr override", true, func(k, v string, s bool) (string, string, string) {
			b, _ := json.Marshal(map[string]any{"key": k, "value": v, "secret": s})
			return http.MethodPost, "/api/v1/apps/web/previews/7/env", string(b)
		}},
	}
	for _, sf := range surfaces {
		t.Run(sf.name, func(t *testing.T) {
			contract.Run(t, 25, func(c contract.Case) (bool, bool) {
				if !c.DefaultPolicyOnly() || len(c.Set) != 1 || c.Options.ExistingUnknown || (c.Options.Secret && !sf.secret) {
					return false, true
				}
				for _, w := range c.WantErrors {
					if w == "crlf" {
						return false, true
					}
				}
				for k, v := range c.Set {
					if k == "" || (c.Options.Secret && v == "") {
						return false, true
					}
					rt, db := newTestRouterWithSecrets(t, &fakeSecretSetter{})
					cookie := loginTestSession(t, rt, db)
					seedDualCopyApp(t, db, c.Existing.Plain, c.Existing.Names...)
					method, path, body := sf.call(k, v, c.Options.Secret)
					rec := httptest.NewRecorder()
					rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
					if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest && rec.Code != http.StatusNoContent && rec.Code != http.StatusConflict {
						t.Errorf("%s: unexpected status %d, body = %s", c.Name, rec.Code, rec.Body.String())
					}
					return rec.Code == http.StatusBadRequest, false
				}
				return false, true
			})
		})
	}
}

// PATH warns by default; every override surface must carry that warning in its response.
func TestOverrideWritersReturnPathWarning(t *testing.T) {
	cases := []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/apps/web/preview-env/PATH", `{"value":"/bin"}`},
		{http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"x/*","key":"PATH","value":"/bin"}`},
		{http.MethodPost, "/api/v1/apps/web/previews/7/env", `{"key":"PATH","value":"/bin"}`},
	}
	for _, c := range cases {
		rt, db := newTestRouterWithSecrets(t, &fakeSecretSetter{})
		cookie := loginTestSession(t, rt, db)
		seedDualCopyApp(t, db, nil)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.path, c.body))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"warnings"`) || !strings.Contains(rec.Body.String(), `"key":"PATH"`) {
			t.Errorf("%s: status %d body %s, want 200 with a PATH warning", c.path, rec.Code, rec.Body.String())
		}
	}
}
