package api

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedFullApp(t *testing.T, db *store.DB) store.DesiredService {
	t.Helper()
	hostPort := 18080
	svc := store.DesiredService{
		Name: "full", Image: "x:1", Port: 8080, HostPort: &hostPort,
		Domains:          []string{"full.example.test"},
		Env:              map[string]string{"MODE": "prod", "DB_PASSWORD": "hunter2"},
		Command:          []string{"server", "--listen", ":8080"},
		Entrypoint:       []string{"/bin/sh", "-c"},
		Labels:           map[string]string{"team": "core"},
		Resources:        &store.ServiceResources{MemoryBytes: 256 << 20, NanoCPUs: 500_000_000},
		Health:           &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/ready"}},
		Hooks:            &store.ServiceHooks{PreDeploy: "migrate"},
		Strategy:         "recreate",
		Replicas:         2,
		DependsOn:        []string{"db"},
		DependsOnHealthy: []string{"db"},
	}
	if err := db.SaveDesiredService(t.Context(), svc); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDesiredService(t.Context(), "full")
	if err != nil {
		t.Fatal(err)
	}
	return *got
}

func putFullApp(t *testing.T, rt *Router, cookie *http.Cookie, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/full", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d body = %s", body, rec.Code, rec.Body.String())
	}
}

func TestAppsPutEnvOnlyKeepsEveryOtherField(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	before := seedFullApp(t, db)

	putFullApp(t, rt, cookie, `{"env":{"MODE":"dev"}}`)
	got, err := db.GetDesiredService(t.Context(), "full")
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["MODE"] != "dev" || len(got.Env) != 1 {
		t.Fatalf("env = %v, want only MODE=dev", got.Env)
	}
	want := before
	want.Env, want.EnvDirty = got.Env, got.EnvDirty
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("env-only PUT changed other fields:\n got %+v\nwant %+v", *got, want)
	}
}

func TestAppsPutExplicitEmptyClears(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFullApp(t, db)

	putFullApp(t, rt, cookie, `{"command":[],"labels":{},"domains":[],"env":{},"resources":null,"health":null,"hooks":null,"host_port":null}`)
	got, err := db.GetDesiredService(t.Context(), "full")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Command) != 0 || len(got.Labels) != 0 || len(got.Domains) != 0 || len(got.Env) != 0 ||
		got.Resources != nil || got.Health != nil || got.Hooks != nil || got.HostPort != nil {
		t.Fatalf("explicit empties did not clear: %+v", *got)
	}
	if !slices.Equal(got.Entrypoint, []string{"/bin/sh", "-c"}) || !slices.Equal(got.DependsOnHealthy, []string{"db"}) {
		t.Fatalf("PUT dropped fields it cannot express: %+v", *got)
	}
}

func TestAppsPutRoundTripIsNoOp(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	before := seedFullApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/full", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body = %s", rec.Code, rec.Body.String())
	}
	putFullApp(t, rt, cookie, rec.Body.String())
	got, err := db.GetDesiredService(t.Context(), "full")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, before) {
		t.Fatalf("GET then PUT changed desired state:\n got %+v\nwant %+v", *got, before)
	}
	// Same applied fields and env means the reconciler sees no drift and recreates nothing.
	if !maps.Equal(store.AppliedFields(*got), store.AppliedFields(before)) || got.EnvDirty {
		t.Fatalf("round trip produced drift: fields %v vs %v, env_dirty %v", store.AppliedFields(*got), store.AppliedFields(before), got.EnvDirty)
	}
}

func TestAppsPutUnicodeFoldedKeysCountAsSent(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFullApp(t, db)

	// A long s (U+017F) and a Kelvin sign (U+212A) fold to "s" and "k", so json decodes them into
	// labels and hooks; they must count as sent and not be refilled from storage.
	putFullApp(t, rt, cookie, `{"label\u017f":{"team":"new"},"hoo\u212as":{"pre_deploy":"other"}}`)
	got, err := db.GetDesiredService(t.Context(), "full")
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels["team"] != "new" {
		t.Fatalf("labels = %v, want team=new", got.Labels)
	}
	if got.Hooks == nil || got.Hooks.PreDeploy != "other" {
		t.Fatalf("hooks = %+v, want pre_deploy other", got.Hooks)
	}
}
