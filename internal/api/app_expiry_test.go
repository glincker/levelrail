package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type expiryAudit struct {
	count   map[string]int
	details map[string][]string
}

func expiryAuditActions(t *testing.T, db *store.DB) expiryAudit {
	t.Helper()
	entries, err := db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	out := expiryAudit{count: map[string]int{}, details: map[string][]string{}}
	for _, e := range entries {
		if !strings.HasPrefix(e.Action, "app.expiry.") {
			continue
		}
		var d map[string]any
		if err := json.Unmarshal([]byte(e.Detail), &d); err != nil || d["app"] == nil {
			t.Fatalf("audit %s detail %q must be JSON naming the app", e.Action, e.Detail)
		}
		out.count[e.Action]++
		out.details[e.Action] = append(out.details[e.Action], e.Detail)
	}
	return out
}

// newExpiryRouter is a test router with the delete switch turned on.
func newExpiryRouter(t *testing.T, deleteEnabled bool) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	if deleteEnabled {
		if rt.runtimeSettings == nil {
			t.Fatal("no runtime settings service")
		}
		if _, err := rt.runtimeSettings.Set(context.Background(), KeyAppExpiryDeleteEnabled, json.RawMessage(`true`), "test"); err != nil {
			t.Fatal(err)
		}
	}
	return rt, db
}

// setExpired stores an already expired row pinned to the app's current life.
func setExpired(t *testing.T, db *store.DB, name string, mod func(*store.AppExpiry)) {
	t.Helper()
	ctx := context.Background()
	inc, err := db.EnsureServiceIncarnation(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	e := store.AppExpiry{AppName: name, ExpiresAt: past, SetAt: past, Incarnation: inc}
	if mod != nil {
		mod(&e)
	}
	if err := db.SetAppExpiry(ctx, e); err != nil {
		t.Fatal(err)
	}
}

func sweepTimes(t *testing.T, rt *Router, times ...time.Time) {
	t.Helper()
	for _, at := range times {
		if _, err := rt.SweepAppExpiry(context.Background(), at); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
}

type failingDeployURLs struct{ *store.DB }

func (failingDeployURLs) ListDeployURLs(context.Context, string) ([]store.DeployURL, error) {
	return nil, errors.New("list unavailable")
}

func TestAppExpiry_ExpiredAppStoppedThenDeletedAfterGrace(t *testing.T) {
	t.Parallel()
	rt, db := newExpiryRouter(t, true)
	ctx := context.Background()
	seedApp(t, db, "demo")
	now := time.Now().UTC()
	setExpired(t, db, "demo", nil)
	sweepTimes(t, rt, now)
	if svc, _ := db.GetDesiredService(ctx, "demo"); svc.Suspended {
		t.Fatal("app stopped before the notice sweep finished")
	}
	sweepTimes(t, rt, now)
	if svc, _ := db.GetDesiredService(ctx, "demo"); !svc.Suspended {
		t.Fatal("expired app was not stopped")
	}
	if n, _ := rt.SweepAppExpiry(ctx, now.Add(time.Hour)); n != 0 {
		t.Fatalf("idempotent sweep acted on %d apps", n)
	}
	if _, err := db.GetDesiredService(ctx, "demo"); err != nil {
		t.Fatalf("app deleted inside grace: %v", err)
	}
	sweepTimes(t, rt, now.Add(8*24*time.Hour))
	if _, err := db.GetDesiredService(ctx, "demo"); err == nil {
		t.Fatal("app not deleted after grace")
	}
	got := expiryAuditActions(t, db)
	for _, a := range []string{"app.expiry.notice", "app.expiry.stop", "app.expiry.delete.begin", "app.expiry.delete"} {
		if got.count[a] != 1 {
			t.Fatalf("audit %s = %d, want 1 (all: %v)", a, got.count[a], got.count)
		}
	}
}

func TestAppExpiry_DeleteSwitchOffOnlyStops(t *testing.T) {
	t.Parallel()
	rt, db := newExpiryRouter(t, false)
	ctx := context.Background()
	seedApp(t, db, "demo")
	setExpired(t, db, "demo", nil)
	now := time.Now()
	sweepTimes(t, rt, now, now, now.Add(30*24*time.Hour))
	svc, err := db.GetDesiredService(ctx, "demo")
	if err != nil || !svc.Suspended {
		t.Fatalf("with the switch off the app must stay, stopped: %v %+v", err, svc)
	}
	e, _ := db.GetAppExpiry(ctx, "demo")
	if res := rt.toAppExpiryResource(ctx, "demo", e); res.DeleteEnabled || res.State != "stopped" || res.DeleteAfter != "" {
		t.Fatalf("resource = %+v, want stopped with delete disabled", res)
	}
}

func TestAppExpiry_KeepRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		svc      store.DesiredService
		mod      func(*store.AppExpiry)
		setup    func(t *testing.T, rt *Router, db *store.DB)
		want     string
		sweepErr bool
	}{
		{name: "volume data", svc: store.DesiredService{Volumes: []store.ServiceVolume{{Name: "app-data", ContainerPath: "/data"}}}, want: "app has persistent volumes"},
		{name: "bind mount", svc: store.DesiredService{BindMounts: []store.ServiceBindMount{{HostPath: "/srv/x", ContainerPath: "/x"}}}, want: "app has bind mounts"},
		{name: "linked database", svc: store.DesiredService{DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "pg", Field: "url"}}}, want: "app has linked databases"},
		{name: "dependents", setup: func(t *testing.T, _ *Router, db *store.DB) {
			if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "front", Image: "img:v1", Port: 80, DependsOn: []string{"app"}}); err != nil {
				t.Fatal(err)
			}
		}, want: "other apps depend on it"},
		{name: "restarted by owner", setup: func(t *testing.T, rt *Router, db *store.DB) {
			sweepTimes(t, rt, time.Now(), time.Now())
			if err := db.UpdateServiceSuspended(context.Background(), "app", false); err != nil {
				t.Fatal(err)
			}
		}, want: store.AppExpiryKeptStarted},
		{name: "restarted then suspended for another reason", setup: func(t *testing.T, rt *Router, db *store.DB) {
			sweepTimes(t, rt, time.Now(), time.Now())
			ctx := context.Background()
			if err := db.UpdateServiceSuspended(ctx, "app", false); err != nil {
				t.Fatal(err)
			}
			if err := db.UpdateServiceSuspended(ctx, "app", true); err != nil {
				t.Fatal(err)
			}
		}, want: store.AppExpiryKeptStarted},
		{name: "delete_data setter lacks write:sensitive", svc: store.DesiredService{Volumes: []store.ServiceVolume{{Name: "app-data", ContainerPath: "/data"}}},
			mod: func(e *store.AppExpiry) {
				e.DeleteData, e.SetterType, e.SetterID = true, store.PrincipalTypeToken, "tok_plain"
			},
			setup: func(t *testing.T, _ *Router, db *store.DB) {
				if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_plain", Name: "p", TokenHash: hashToken("lr_plain_setter"), Abilities: []string{AbilityWrite}, CreatedAt: time.Now()}); err != nil {
					t.Fatal(err)
				}
			}, want: appExpirySetterGone},
		{name: "delete_data setter token removed", mod: func(e *store.AppExpiry) {
			e.DeleteData, e.SetterType, e.SetterID = true, store.PrincipalTypeToken, "tok_missing"
		}, want: appExpirySetterGone},
		{name: "retained list error retries, never kept", setup: func(_ *testing.T, rt *Router, db *store.DB) {
			rt.deployAttempts = failingDeployURLs{db}
		}, want: "", sweepErr: true},
		{name: "protected environment", setup: func(t *testing.T, _ *Router, db *store.DB) {
			ctx := context.Background()
			if err := db.SaveProject(ctx, store.Project{ID: "p1", Name: "p1", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
				t.Fatal(err)
			}
			if err := db.SaveEnvironment(ctx, store.Environment{ID: "env-prod", ProjectID: "p1", Name: "production", Protected: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
				t.Fatal(err)
			}
			if err := db.SetServiceEnvironment(ctx, "app", "env-prod"); err != nil {
				t.Fatal(err)
			}
		}, want: "app is in a protected environment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt, db := newExpiryRouter(t, true)
			ctx := context.Background()
			svc := tc.svc
			svc.Name, svc.Image, svc.Port = "app", "img:v1", 3000
			if err := db.SaveDesiredService(ctx, svc); err != nil {
				t.Fatal(err)
			}
			setExpired(t, db, "app", tc.mod)
			if tc.setup != nil {
				tc.setup(t, rt, db)
			}
			now := time.Now()
			var sweepErr error
			for _, at := range []time.Time{now, now, now.Add(30 * 24 * time.Hour), now.Add(31 * 24 * time.Hour)} {
				if _, err := rt.SweepAppExpiry(ctx, at); err != nil {
					sweepErr = err
				}
			}
			if (sweepErr != nil) != tc.sweepErr {
				t.Fatalf("sweep error = %v, want error %v", sweepErr, tc.sweepErr)
			}
			if _, err := db.GetDesiredService(ctx, "app"); err != nil {
				t.Fatalf("app was deleted: %v", err)
			}
			e, _ := db.GetAppExpiry(ctx, "app")
			if e == nil || e.KeptReason != tc.want {
				t.Fatalf("kept reason = %+v, want %q", e, tc.want)
			}
			got := expiryAuditActions(t, db)
			if got.count["app.expiry.delete.begin"] != 0 {
				t.Fatalf("delete began for a kept app: %v", got.count)
			}
			if tc.want != "" && tc.want != store.AppExpiryKeptStarted {
				if got.count["app.expiry.kept"] != 1 || !strings.Contains(got.details["app.expiry.kept"][0], tc.want) {
					t.Fatalf("kept audit = %v, want one row naming %q", got.details, tc.want)
				}
			}
		})
	}
}

func TestAppExpiry_DeleteDataWithAuthorizedSetter(t *testing.T) {
	t.Parallel()
	rt, db := newExpiryRouter(t, true)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "tmp", Image: "img:v1", Port: 3000,
		Volumes: []store.ServiceVolume{{Name: "tmp-data", ContainerPath: "/data"}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "tok_admin", Name: "a", TokenHash: hashToken("lr_admin_setter"), Abilities: []string{AbilityWrite, AbilityWriteSensitive}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	setExpired(t, db, "tmp", func(e *store.AppExpiry) {
		e.DeleteData, e.SetterType, e.SetterID = true, store.PrincipalTypeToken, "tok_admin"
	})
	now := time.Now()
	sweepTimes(t, rt, now, now, now.Add(8*24*time.Hour))
	if _, err := db.GetDesiredService(ctx, "tmp"); err == nil {
		t.Fatal("app with delete_data and an authorized setter was not deleted after grace")
	}
}

func TestAppExpiry_IdentityAndRaces(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name string
		run  func(t *testing.T, rt *Router, db *store.DB)
	}{
		{"recreated app does not inherit and the stale row is cleared", func(t *testing.T, rt *Router, db *store.DB) {
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			old, _ := db.GetAppExpiry(ctx, "app")
			if err := db.DeleteDesiredService(ctx, "app"); err != nil {
				t.Fatal(err)
			}
			if got, _ := db.GetAppExpiry(ctx, "app"); got != nil {
				t.Fatal("DeleteDesiredService left the expiry row")
			}
			seedApp(t, db, "app")
			if err := db.SetAppExpiry(ctx, *old); err != nil { // a row from the old life
				t.Fatal(err)
			}
			sweepTimes(t, rt, time.Now(), time.Now(), time.Now().Add(30*24*time.Hour))
			if svc, err := db.GetDesiredService(ctx, "app"); err != nil || svc.Suspended {
				t.Fatalf("recreated app was touched: %v %+v", err, svc)
			}
			if got, _ := db.GetAppExpiry(ctx, "app"); got != nil {
				t.Fatal("stale row from the old life was not cleared")
			}
		}},
		{"deleteApp clears the row", func(t *testing.T, rt *Router, db *store.DB) {
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			if _, err := rt.deleteApp(ctx, "app"); err != nil {
				t.Fatal(err)
			}
			if got, _ := db.GetAppExpiry(ctx, "app"); got != nil {
				t.Fatal("deleteApp left the expiry row")
			}
		}},
		{"clear between read and delete cancels the delete", func(t *testing.T, rt *Router, db *store.DB) {
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			sweepTimes(t, rt, time.Now(), time.Now())
			e, _ := db.GetAppExpiry(ctx, "app")
			if _, err := db.ClearAppExpiry(ctx, "app"); err != nil {
				t.Fatal(err)
			}
			if did, err := rt.deleteExpiredApp(ctx, db, *e); err != nil || did {
				t.Fatalf("stale snapshot deleted the app: did=%v err=%v", did, err)
			}
			if _, err := db.GetDesiredService(ctx, "app"); err != nil {
				t.Fatalf("app deleted: %v", err)
			}
		}},
		{"released claim never brings back a cleared row", func(t *testing.T, _ *Router, db *store.DB) {
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			e, _ := db.GetAppExpiry(ctx, "app")
			if ok, err := db.ClaimAppExpiry(ctx, *e, "c1"); err != nil || !ok {
				t.Fatalf("claim: %v %v", ok, err)
			}
			if ok, _ := db.ClaimAppExpiry(ctx, *e, "c2"); ok {
				t.Fatal("second claim won")
			}
			if _, err := db.ClearAppExpiry(ctx, "app"); err != nil {
				t.Fatal(err)
			}
			if err := db.ReleaseAppExpiryClaim(ctx, "app", "c1"); err != nil {
				t.Fatal(err)
			}
			if got, _ := db.GetAppExpiry(ctx, "app"); got != nil {
				t.Fatalf("cleared row came back: %+v", got)
			}
		}},
		{"stop stamp rolls back", func(t *testing.T, _ *Router, db *store.DB) {
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			e, _ := db.GetAppExpiry(ctx, "app")
			if ok, err := db.MarkAppExpiry(ctx, *e, "n", "s1", "sweep", ""); err != nil || !ok {
				t.Fatalf("mark: %v %v", ok, err)
			}
			if err := db.UnstampAppExpiryStop(ctx, "app", "s1"); err != nil {
				t.Fatal(err)
			}
			if got, _ := db.GetAppExpiry(ctx, "app"); got.StoppedAt != "" || got.StoppedBy != "" {
				t.Fatalf("stop stamp not rolled back: %+v", got)
			}
		}},
		{"running deploy is skipped", func(t *testing.T, rt *Router, db *store.DB) {
			rt.deploySafety = db
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "att1", ServiceName: "app", Image: "img:v1", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			if n, _ := rt.SweepAppExpiry(ctx, time.Now()); n != 0 {
				t.Fatalf("sweep acted on an app with a running deploy: %d", n)
			}
			if e, _ := db.GetAppExpiry(ctx, "app"); e == nil || e.NotifiedAt != "" {
				t.Fatalf("row advanced during a deploy: %+v", e)
			}
		}},
		{"incarnation is claimed once", func(t *testing.T, _ *Router, db *store.DB) {
			seedApp(t, db, "app")
			if _, err := db.ExecContext(ctx, `UPDATE desired_services SET incarnation = '' WHERE name = 'app'`); err != nil {
				t.Fatal(err)
			}
			id, updated, err := db.ClaimServiceIncarnation(ctx, "app")
			if err != nil || !updated || id == "" {
				t.Fatalf("first claim: %q %v %v", id, updated, err)
			}
			if _, updated, _ := db.ClaimServiceIncarnation(ctx, "app"); updated {
				t.Fatal("second claim replaced the id")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt, db := newExpiryRouter(t, true)
			tc.run(t, rt, db)
		})
	}
}

func TestAppExpiry_ClearCancelsPendingDeleteAndSweepIsBounded(t *testing.T) {
	t.Parallel()
	rt, db := newExpiryRouter(t, true)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, n := range []string{"a1", "a2", "a3"} {
		seedApp(t, db, n)
		setExpired(t, db, n, nil)
	}
	if _, err := rt.runtimeSettings.Set(ctx, KeyAppExpiryMaxPerSweep, json.RawMessage(`2`), "test"); err != nil {
		t.Fatal(err)
	}
	if n, _ := rt.SweepAppExpiry(ctx, now); n != 2 {
		t.Fatalf("bounded sweep acted on %d, want 2", n)
	}
	for range 4 {
		_, _ = rt.SweepAppExpiry(ctx, now)
	}
	if _, err := db.ClearAppExpiry(ctx, "a1"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_, _ = rt.SweepAppExpiry(ctx, now.Add(10*24*time.Hour))
	}
	if _, err := db.GetDesiredService(ctx, "a1"); err != nil {
		t.Fatalf("cleared app was deleted: %v", err)
	}
	if _, err := db.GetDesiredService(ctx, "a2"); err == nil {
		t.Fatal("uncleared expired app was not deleted")
	}
}

func TestAppExpiry_SetValidatesAndClearCancels(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	cases := []struct {
		method, target, body string
		want                 int
	}{
		{http.MethodPut, "/api/v1/apps/web/expiry", `{"expires_at":"2000-01-01T00:00:00Z"}`, http.StatusBadRequest},
		{http.MethodPut, "/api/v1/apps/web/expiry", `{"expires_at":"` + time.Now().Add(5000*24*time.Hour).UTC().Format(time.RFC3339) + `"}`, http.StatusBadRequest},
		{http.MethodPut, "/api/v1/apps/web/expiry", `{"expires_at":"` + future + `","delete_data":true}`, http.StatusBadRequest},
		{http.MethodPut, "/api/v1/apps/missing/expiry", `{"expires_at":"` + future + `"}`, http.StatusNotFound},
		{http.MethodPut, "/api/v1/apps/web/expiry", `{"expires_at":"` + future + `"}`, http.StatusOK},
		{http.MethodDelete, "/api/v1/apps/web/expiry", "", http.StatusOK},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.target, c.body))
		if rec.Code != c.want {
			t.Fatalf("%s %s %s = %d, want %d: %s", c.method, c.target, c.body, rec.Code, c.want, rec.Body.String())
		}
	}
	if e, _ := db.GetAppExpiry(context.Background(), "web"); e != nil {
		t.Fatalf("clear left the expiry: %+v", e)
	}
}

func TestAppExpiry_SetAuthz(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	cases := []struct {
		name      string
		abilities []string
		body      string
		want      int
	}{
		{"deploy only token cannot set", []string{AbilityRead, AbilityDeploy}, `{"expires_at":"` + future + `"}`, http.StatusForbidden},
		{"write token without write:sensitive cannot set delete_data", []string{AbilityRead, AbilityWrite}, `{"expires_at":"` + future + `","delete_data":true,"confirm":"web"}`, http.StatusForbidden},
		{"write token can set a plain expiry", []string{AbilityRead, AbilityWrite}, `{"expires_at":"` + future + `"}`, http.StatusOK},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt, db := newTestRouter(t)
			seedApp(t, db, "web")
			plaintext := "lr_test_expiry_token_" + strconv.Itoa(i)
			tokenID := "tok_exp" + strconv.Itoa(i)
			if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: tokenID, Name: "t", TokenHash: hashToken(plaintext), Abilities: tc.abilities, CreatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/expiry", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+plaintext)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body.String(), tc.want)
			}
			if tc.want != http.StatusOK {
				return
			}
			e, _ := db.GetAppExpiry(context.Background(), "web")
			if e == nil || e.SetterType != store.PrincipalTypeToken || e.SetterID != tokenID {
				t.Fatalf("setter not recorded: %+v", e)
			}
			got := expiryAuditActions(t, db)
			if got.count["app.expiry.set"] != 1 || !strings.Contains(got.details["app.expiry.set"][0], `"delete_data":false`) {
				t.Fatalf("set audit = %v, want one row with delete_data", got.details)
			}
		})
	}
}

func TestAppExpiry_ThirdReviewGuards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	protectedEnv := func(t *testing.T, db *store.DB) {
		t.Helper()
		if err := db.SaveProject(ctx, store.Project{ID: "p1", Name: "p1", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveEnvironment(ctx, store.Environment{ID: "env-prod", ProjectID: "p1", Name: "production", Protected: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetServiceEnvironment(ctx, "app", "env-prod"); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name string
		run  func(t *testing.T, rt *Router, db *store.DB)
	}{
		{"protected environment keeps even with delete_data", func(t *testing.T, rt *Router, db *store.DB) {
			if err := db.SaveAPIToken(ctx, store.APIToken{ID: "tok_admin", Name: "a", TokenHash: hashToken("lr_admin_env"), Abilities: []string{AbilityWrite, AbilityWriteSensitive}, CreatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			seedApp(t, db, "app")
			protectedEnv(t, db)
			setExpired(t, db, "app", func(e *store.AppExpiry) {
				e.DeleteData, e.SetterType, e.SetterID = true, store.PrincipalTypeToken, "tok_admin"
			})
			now := time.Now()
			sweepTimes(t, rt, now, now, now.Add(30*24*time.Hour))
			svc, err := db.GetDesiredService(ctx, "app")
			if err != nil || svc.Suspended {
				t.Fatalf("protected environment app was stopped or deleted: %v %+v", err, svc)
			}
			if e, _ := db.GetAppExpiry(ctx, "app"); e == nil || e.KeptReason != "app is in a protected environment" {
				t.Fatalf("kept = %+v", e)
			}
		}},
		{"PUT refuses an app in a protected environment", func(t *testing.T, rt *Router, db *store.DB) {
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "app")
			protectedEnv(t, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/app/expiry", `{"expires_at":"`+time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339)+`"}`))
			if rec.Code != http.StatusConflict {
				t.Fatalf("got %d %s, want 409", rec.Code, rec.Body.String())
			}
		}},
		{"disabled user setter keeps the app", func(t *testing.T, rt *Router, db *store.DB) {
			u := storeUserWithAbilitiesForTest(t, db, "setter@example.test", []string{AbilityWrite, AbilityWriteSensitive})
			if rt.authEngine == nil {
				t.Skip("no auth engine")
			}
			if err := rt.authEngine.DisableUser(ctx, u.ID); err != nil {
				t.Skipf("cannot disable a store-only user here: %v", err)
			}
			if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "app", Image: "img:v1", Port: 3000, Volumes: []store.ServiceVolume{{Name: "app-data", ContainerPath: "/data"}}}); err != nil {
				t.Fatal(err)
			}
			setExpired(t, db, "app", func(e *store.AppExpiry) {
				e.DeleteData, e.SetterType, e.SetterID = true, store.PrincipalTypeUser, u.ID
			})
			now := time.Now()
			sweepTimes(t, rt, now, now, now.Add(30*24*time.Hour))
			if e, _ := db.GetAppExpiry(ctx, "app"); e == nil || e.KeptReason != appExpirySetterGone {
				t.Fatalf("kept = %+v", e)
			}
		}},
		{"PUT is refused while a delete holds the claim", func(t *testing.T, rt *Router, db *store.DB) {
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			e, _ := db.GetAppExpiry(ctx, "app")
			if ok, err := db.ClaimAppExpiry(ctx, *e, time.Now().UTC().Format(time.RFC3339Nano)); err != nil || !ok {
				t.Fatalf("claim: %v %v", ok, err)
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/app/expiry", `{"expires_at":"`+time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339)+`"}`))
			if rec.Code != http.StatusConflict {
				t.Fatalf("got %d %s, want 409", rec.Code, rec.Body.String())
			}
		}},
		{"delete switch is re-read under the lock", func(t *testing.T, rt *Router, db *store.DB) {
			seedApp(t, db, "app")
			setExpired(t, db, "app", nil)
			sweepTimes(t, rt, time.Now(), time.Now())
			if _, err := rt.runtimeSettings.Set(ctx, KeyAppExpiryDeleteEnabled, json.RawMessage(`false`), "test"); err != nil {
				t.Fatal(err)
			}
			e, _ := db.GetAppExpiry(ctx, "app")
			if did, err := rt.deleteExpiredApp(ctx, db, *e); err != nil || did {
				t.Fatalf("delete ran with the switch off: %v %v", did, err)
			}
			if _, err := db.GetDesiredService(ctx, "app"); err != nil {
				t.Fatalf("app deleted: %v", err)
			}
		}},
		{"stale claim is released, fresh claim is kept", func(t *testing.T, rt *Router, db *store.DB) {
			for _, n := range []string{"old", "new"} {
				seedApp(t, db, n)
				setExpired(t, db, n, nil)
			}
			now := time.Now().UTC()
			for n, at := range map[string]time.Time{"old": now.Add(-2 * time.Hour), "new": now} {
				e, _ := db.GetAppExpiry(ctx, n)
				if ok, err := db.ClaimAppExpiry(ctx, *e, at.Format(time.RFC3339Nano)); err != nil || !ok {
					t.Fatalf("claim %s: %v %v", n, ok, err)
				}
			}
			sweepTimes(t, rt, now)
			if e, _ := db.GetAppExpiry(ctx, "old"); e.ClaimedAt != "" {
				t.Fatalf("stale claim not released: %+v", e)
			}
			if e, _ := db.GetAppExpiry(ctx, "new"); e.ClaimedAt == "" {
				t.Fatal("fresh claim was released")
			}
			if got := expiryAuditActions(t, db); got.count["app.expiry.claim.released"] != 1 {
				t.Fatalf("claim release audit = %v", got.count)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt, db := newExpiryRouter(t, true)
			tc.run(t, rt, db)
		})
	}
}
