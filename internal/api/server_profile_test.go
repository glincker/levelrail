package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/profile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const profileDiskBytes = int64(100) << 30

func newProfileRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	WithServerProfile(db)(rt)
	rt.serverProfile.lookupEnv = func(string) (string, bool) { return "", false }
	rt.serverProfile.diskBytes = func() int64 { return profileDiskBytes }
	profile.SetActive(profile.Standard, profile.SourceDefault)
	t.Cleanup(func() { profile.SetActive(profile.Standard, profile.SourceDefault) })
	return rt, db, loginTestSession(t, rt, db)
}

func profileDo(t *testing.T, rt *Router, cookie *http.Cookie, method, body string) (*httptest.ResponseRecorder, serverProfileResource) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/settings/server-profile", body))
	var res serverProfileResource
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	return rec, res
}

func TestServerProfile_SaveAppliesAfterRestart(t *testing.T) {
	rt, db, cookie := newProfileRouter(t)

	rec, res := profileDo(t, rt, cookie, http.MethodGet, "")
	if rec.Code != http.StatusOK || res.Configured != "standard" || res.Source != profile.SourceDefault || res.RestartRequired || len(res.Profiles) != 3 {
		t.Fatalf("GET = %d %+v", rec.Code, res)
	}

	rec, res = profileDo(t, rt, cookie, http.MethodPut, `{"profile":"suite"}`)
	if rec.Code != http.StatusOK || res.Configured != "suite" || res.Source != profile.SourceSetting || !res.RestartRequired || res.Active != "standard" {
		t.Fatalf("PUT = %d %+v", rec.Code, res)
	}
	if s, _ := db.GetServerProfile(t.Context()); s.Profile != "suite" || !s.Set {
		t.Fatalf("stored = %+v", s)
	}
}

// The profile is saved but nothing can restart the process: the state must still say a restart is owed.
func TestServerProfile_SavedButRestartRefused(t *testing.T) {
	rt, _, cookie := newProfileRouter(t)
	if rec, _ := profileDo(t, rt, cookie, http.MethodPut, `{"profile":"suite"}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/restart", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("unsupervised restart = %d, want 409", rec.Code)
	}
	_, res := profileDo(t, rt, cookie, http.MethodGet, "")
	if res.Configured != "suite" || !res.RestartRequired {
		t.Fatalf("after refused restart = %+v", res)
	}
}

func TestServerProfile_DownwardGuards(t *testing.T) {
	rt, db, cookie := newProfileRouter(t)
	ctx := context.Background()

	rec, res := profileDo(t, rt, cookie, http.MethodPut, `{"profile":"minimal"}`)
	if rec.Code != http.StatusConflict || !res.NeedsConfirm || res.Error == "" {
		t.Fatalf("shorter retention without confirm = %d %+v", rec.Code, res)
	}
	if s, _ := db.GetServerProfile(ctx); s.Set {
		t.Fatal("a refused move must not persist")
	}
	if rec, _ = profileDo(t, rt, cookie, http.MethodPut, `{"profile":"minimal","dry_run":true,"confirm":true}`); rec.Code != http.StatusOK {
		t.Fatalf("dry run = %d", rec.Code)
	}
	if s, _ := db.GetServerProfile(ctx); s.Set {
		t.Fatal("a dry run must not persist")
	}

	if err := db.SaveNode(ctx, store.Node{ID: "n1", Name: "edge", Address: "10.0.0.2:9443", Status: store.NodeStatusOnline, Schedulable: true, AcceptsAppWorkloads: true}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	rec, res = profileDo(t, rt, cookie, http.MethodPut, `{"profile":"minimal","confirm":true}`)
	if rec.Code != http.StatusConflict || len(res.Problems) == 0 || res.Problems[0].Code != profile.GuardNodesEnrolled {
		t.Fatalf("enrolled node = %d %+v", rec.Code, res)
	}

	if rec, _ = profileDo(t, rt, cookie, http.MethodPut, `{"profile":"suite"}`); rec.Code != http.StatusOK {
		t.Fatalf("moving up with a node = %d", rec.Code)
	}
}

func TestServerProfile_PinnedByEnvAndInvalid(t *testing.T) {
	rt, _, cookie := newProfileRouter(t)
	rt.serverProfile.lookupEnv = func(k string) (string, bool) {
		if k == profile.EnvVar {
			return "suite", true
		}
		return "", false
	}
	rec, _ := profileDo(t, rt, cookie, http.MethodPut, `{"profile":"standard"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("env pinned = %d %s", rec.Code, rec.Body.String())
	}
	if _, g := profileDo(t, rt, cookie, http.MethodGet, ""); !g.PinnedByEnv || g.Configured != "suite" || g.Source != profile.SourceEnv {
		t.Fatalf("GET = %+v", g)
	}
	rt.serverProfile.lookupEnv = func(string) (string, bool) { return "", false }
	if rec, _ = profileDo(t, rt, cookie, http.MethodPut, `{"profile":"huge"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid profile = %d", rec.Code)
	}
}

func TestServerProfile_OnlyRootMayChange(t *testing.T) {
	rt, _, cookie := newProfileRouter(t)
	tok := mintTokenAs(t, rt, cookie, "profile-reader")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/settings/server-profile", "", tok.Token))
	if rec.Code != http.StatusOK {
		t.Fatalf("read token GET = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPut, "/api/v1/settings/server-profile", `{"profile":"suite"}`, tok.Token))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read token PUT = %d, want 403", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/settings/server-profile", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous PUT = %d, want 401", rec.Code)
	}
}

func TestPostureRecommendationsDoNotLowerScoreOnMinimal(t *testing.T) {
	t.Cleanup(func() { profile.SetActive(profile.Standard, profile.SourceDefault) })
	for _, c := range []struct {
		p            profile.Profile
		offbox       bool
		wantScoreCut int
	}{{profile.Standard, false, 15}, {profile.Minimal, true, 0}} {
		profile.SetActive(c.p, profile.SourceDefault)
		in := healthyPosture()
		off := false
		in.OffBoxBackups = &off
		in.AppsLive = true
		in.MasterKey, in.MasterKeyKnown, in.MasterKeyRotated = true, true, nil
		items := evaluatePosture(in)
		got := map[string]postureItem{}
		for _, it := range items {
			got[it.ID] = it
		}
		if got["offbox_backup_off"].Status != postureStatusFail || got["offbox_backup_off"].Recommended != c.offbox {
			t.Errorf("%s offbox_backup_off = %+v, want failing with recommended %v", c.p, got["offbox_backup_off"], c.offbox)
		}
		if got["master_key_rotation"].Status != postureStatusFail || !got["master_key_rotation"].Recommended {
			t.Errorf("%s master_key_rotation = %+v, want failing and recommended", c.p, got["master_key_rotation"])
		}
		if s := postureScore(items); s != 100-c.wantScoreCut {
			t.Errorf("%s score = %d, want %d", c.p, s, 100-c.wantScoreCut)
		}
	}
}
