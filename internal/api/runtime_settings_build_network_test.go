package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestBuildNetworkSetting_WeakeningNeedsConfirm(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	path := rsBase + "/" + build.KeyBuildNetwork
	steps := []struct {
		body string
		code int
	}{
		{`{"value":"block-private"}`, http.StatusOK},
		{`{"value":"none"}`, http.StatusOK},
		{`{"value":"block-private"}`, http.StatusBadRequest},
		{`{"value":"open"}`, http.StatusBadRequest},
		{`{"value":"open","confirm_weakening":true}`, http.StatusOK},
		{`{"value":"nope"}`, http.StatusBadRequest},
	}
	for _, st := range steps {
		if rec := rsDo(t, rt, cookie, http.MethodPut, path, st.body); rec.Code != st.code {
			t.Fatalf("PUT %s = %d %s, want %d", st.body, rec.Code, rec.Body.String(), st.code)
		}
	}
}

func TestBuildNetworkSetting_FailsClosed(t *testing.T) {
	_, db := newTestRouter(t)
	ctx := t.Context()
	env := map[string]string{}
	svc := runtimesettings.New(db, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	key := build.KeyBuildNetwork

	env[build.EnvBuildNetwork] = "NONE"
	if got, err := svc.StringChecked(ctx, key); err != nil || got != "none" {
		t.Fatalf("upper case env = %q, %v, want none", got, err)
	}
	env[build.EnvBuildNetwork] = "nnone"
	if _, err := svc.StringChecked(ctx, key); err == nil || !strings.Contains(err.Error(), build.EnvBuildNetwork) {
		t.Fatalf("typo env err = %v, want it to name %s", err, build.EnvBuildNetwork)
	}

	if err := db.SetRuntimeSetting(ctx, key, "open", "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	svc = runtimesettings.New(db, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if got, err := svc.StringChecked(ctx, key); err != nil || got != "open" {
		t.Fatalf("valid saved value must win over a bad env, got %q, %v", got, err)
	}

	if err := db.SetRuntimeSetting(ctx, key, "bogus", "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	svc = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	if _, err := svc.StringChecked(ctx, key); err == nil || !strings.Contains(err.Error(), "saved setting") {
		t.Fatalf("invalid saved value err = %v, want it to name the saved setting", err)
	}
}
