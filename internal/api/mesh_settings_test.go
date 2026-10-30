package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeMeshProbe struct{ privileged bool }

func (fakeMeshProbe) OS() string               { return "linux" }
func (fakeMeshProbe) KernelModuleLoaded() bool { return true }
func (p fakeMeshProbe) Privileged() bool       { return p.privileged }

func meshReadyEnv(failWG bool) func(bool) network.ReadinessEnv {
	return func(running bool) network.ReadinessEnv {
		return network.ReadinessEnv{
			Probe:            fakeMeshProbe{privileged: !failWG},
			TunDevicePresent: func() bool { return true },
			UDPPortFree:      func(int) (bool, error) { return true, nil },
			UFWActive:        func() bool { return false },
			MeshRunning:      running,
			AdvertiseHost:    "203.0.113.5",
		}
	}
}

func meshSettingsDo(t *testing.T, rt *Router, cookie *http.Cookie, method, body string) (*httptest.ResponseRecorder, meshSettingsResource) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/mesh/settings", body))
	var res meshSettingsResource
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	return rec, res
}

func newMeshSettingsRouter(t *testing.T, failWG bool) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	WithMeshSettings(db)(rt)
	rt.meshSettings.lookupEnv = func(string) (string, bool) { return "", false }
	rt.meshSettings.readiness = meshReadyEnv(failWG)
	return rt, db, loginTestSession(t, rt, db)
}

func TestMeshSettings_EnablePreconditionsAndPersistence(t *testing.T) {
	rt, db, cookie := newMeshSettingsRouter(t, true)

	rec, res := meshSettingsDo(t, rt, cookie, http.MethodGet, "")
	if rec.Code != http.StatusOK || res.CanEnable || res.Source != network.SettingSourceDefault {
		t.Fatalf("GET = %d %+v", rec.Code, res)
	}
	rec, _ = meshSettingsDo(t, rt, cookie, http.MethodPut, `{"enabled":true}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "wireguard") {
		t.Fatalf("enable with failing check = %d %s", rec.Code, rec.Body.String())
	}
	if s, _ := db.GetMeshSetting(t.Context()); s.Set {
		t.Fatal("a refused enable must not persist")
	}

	rt.meshSettings.readiness = meshReadyEnv(false)
	rec, res = meshSettingsDo(t, rt, cookie, http.MethodPut, `{"enabled":true}`)
	if rec.Code != http.StatusOK || !res.Configured || res.Source != network.SettingSourceSettings || !res.RestartRequired || res.RestartHint == "" {
		t.Fatalf("enable = %d %+v", rec.Code, res)
	}
}

func TestMeshSettings_EnvPinWins(t *testing.T) {
	rt, db, cookie := newMeshSettingsRouter(t, false)
	rt.meshSettings.lookupEnv = func(k string) (string, bool) { return "1", k == network.EnvMeshEnabled }
	if err := db.SetMeshSetting(t.Context(), false, "x", time.Now()); err != nil {
		t.Fatal(err)
	}

	_, res := meshSettingsDo(t, rt, cookie, http.MethodGet, "")
	if !res.Configured || res.Source != network.SettingSourceEnv {
		t.Fatalf("env pin should win over saved false: %+v", res)
	}
	rec, _ := meshSettingsDo(t, rt, cookie, http.MethodPut, `{"enabled":false}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("PUT while pinned = %d", rec.Code)
	}
}

func TestMeshSettings_DisableGuardsEnrolledNodes(t *testing.T) {
	rt, db, cookie := newMeshSettingsRouter(t, false)
	seedNode(t, db, "node_a", "alpha")
	if err := db.SetMeshSetting(t.Context(), true, "x", time.Now()); err != nil {
		t.Fatal(err)
	}

	rec, _ := meshSettingsDo(t, rt, cookie, http.MethodPut, `{"enabled":false}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("disable with enrolled node = %d", rec.Code)
	}
	rec, res := meshSettingsDo(t, rt, cookie, http.MethodPut, `{"enabled":false,"force":true}`)
	if rec.Code != http.StatusOK || res.Configured {
		t.Fatalf("forced disable = %d %+v", rec.Code, res)
	}
}

func TestMeshSettings_UpdateIsRootOnly(t *testing.T) {
	rt, db := newTestRouter(t)
	WithMeshSettings(db)(rt)
	bootstrapTestAdmin(t, db)
	rt.meshSettings.lookupEnv = func(string) (string, bool) { return "", false }
	rt.meshSettings.readiness = meshReadyEnv(false)
	user := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, user.ID)

	if rec, _ := meshSettingsDo(t, rt, cookie, http.MethodGet, ""); rec.Code != http.StatusOK {
		t.Fatalf("read-only GET = %d", rec.Code)
	}
	if rec, _ := meshSettingsDo(t, rt, cookie, http.MethodPut, `{"enabled":true}`); rec.Code != http.StatusForbidden {
		t.Fatalf("read-only PUT = %d, want 403", rec.Code)
	}
}

func TestTopologyResponse_GroupsLocalWhenMeshOff(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := t.Context()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "w:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/topology", ""))
	var got networkTopologyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Kind != "local" || len(got.Groups[0].Apps) != 1 || len(got.Groups[0].Databases) != 1 {
		t.Fatalf("groups = %+v, want one local group holding both resources", got.Groups)
	}
}

func TestMeshSettings_ReportsAgentAddr(t *testing.T) {
	rt, _, cookie := newMeshSettingsRouter(t, false)
	rt.doctorAgentAdvertiseHost = "203.0.113.5"
	rt.doctorAgentAdvertisePort = 9443
	_, res := meshSettingsDo(t, rt, cookie, http.MethodGet, "")
	if res.AgentAddr != "203.0.113.5:9443" {
		t.Fatalf("agent_addr = %q, want the advertise address agents dial", res.AgentAddr)
	}
}
