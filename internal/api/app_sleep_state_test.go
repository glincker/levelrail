package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakePortWaker struct {
	supported bool
	status    appsleep.PortWakeStatus
}

func (f fakePortWaker) Supported(*store.DesiredService) (bool, string) {
	return f.supported, "the fake says no"
}
func (f fakePortWaker) Status(string) appsleep.PortWakeStatus { return f.status }

func sleepGet(t *testing.T, rt *Router, name string) appSleepResource {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/"+name+"/sleep", nil)
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	rt.handleGetAppSleep(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get sleep = %d %s", rec.Code, rec.Body)
	}
	var res appSleepResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAppSleepStateMatrix(t *testing.T) {
	port := 3000
	tests := []struct {
		name        string
		svc         store.DesiredService
		pw          appsleep.PortWaker
		body        string
		wantCode    int
		wantBlocker string
		wantFix     string
	}{
		{"pinned port without port wake runtime", store.DesiredService{Name: "web", Image: "grafana/grafana", Port: 3000, HostPort: &port}, nil, `{"idle_minutes":30}`, http.StatusConflict, appsleep.CodePinnedPort, appsleep.FixRemovePinnedPort},
		{"pinned port with port wake", store.DesiredService{Name: "web", Image: "grafana/grafana", Port: 3000, HostPort: &port}, fakePortWaker{supported: true}, `{"idle_minutes":30}`, http.StatusOK, "", ""},
		{"pinned port, port wake switched off", store.DesiredService{Name: "web", Image: "grafana/grafana", Port: 3000, HostPort: &port}, fakePortWaker{supported: true}, `{"idle_minutes":30,"port_wake":false}`, http.StatusConflict, appsleep.CodePinnedPort, appsleep.FixEnablePortWake},
		{"bind failure", store.DesiredService{Name: "web", Image: "grafana/grafana", Port: 3000, HostPort: &port}, fakePortWaker{supported: true, status: appsleep.PortWakeStatus{Error: "address in use"}}, `{"idle_minutes":30}`, http.StatusConflict, appsleep.CodePortInUse, appsleep.FixRemovePinnedPort},
		{"plain app", store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Domains: []string{"web.example.com"}}, nil, `{"idle_minutes":30}`, http.StatusOK, "", ""},
		{"bad ignore path", store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}, nil, `{"idle_minutes":30,"ignore_paths":["healthz"]}`, http.StatusBadRequest, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
			rt.sleepPortWake = tc.pw
			if err := db.SaveDesiredService(context.Background(), tc.svc); err != nil {
				t.Fatal(err)
			}
			rec := sleepPut(t, rt.handleSetAppSleep, tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d: %s", rec.Code, tc.wantCode, rec.Body)
			}
			if tc.wantBlocker == "" {
				return
			}
			var ref sleepRefusal
			if err := json.Unmarshal(rec.Body.Bytes(), &ref); err != nil {
				t.Fatal(err)
			}
			if len(ref.Blockers) == 0 || ref.Blockers[0].Code != tc.wantBlocker || ref.Blockers[0].Fix == nil || ref.Blockers[0].Fix.Kind != tc.wantFix {
				t.Fatalf("blockers = %+v, want %s with fix %s", ref.Blockers, tc.wantBlocker, tc.wantFix)
			}
			if a, _ := db.GetAppSleep(context.Background(), "web"); a.IdleMinutes != 0 {
				t.Error("a refused request stored a sleep setting")
			}
		})
	}
}

func TestAppSleepGetContract(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	port := 3000
	rt.sleepPortWake = fakePortWaker{supported: true}
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "grafana/grafana", Port: 3000, HostPort: &port, Resources: &store.ServiceResources{MemoryBytes: 512 << 20}}); err != nil {
		t.Fatal(err)
	}
	res := sleepGet(t, rt, "web")
	if !res.Eligible || len(res.Blockers) != 0 || res.BlockedReason != "" {
		t.Fatalf("pinned app with port wake must be eligible: %+v", res)
	}
	codes := map[string]bool{}
	for _, w := range res.Warnings {
		codes[w.Code] = true
	}
	for _, want := range []string{appsleep.CodeNoDomain, appsleep.CodeDirectTraffic, appsleep.CodeBackgroundWork} {
		if !codes[want] {
			t.Errorf("missing warning %s in %v", want, codes)
		}
	}
	if !res.PortWake.Supported || len(res.PortWake.Ports) != 1 || res.PortWake.Ports[0] != 3000 {
		t.Errorf("port_wake = %+v", res.PortWake)
	}
	if res.EstimatedMemoryFreedBytes == nil || *res.EstimatedMemoryFreedBytes != 512<<20 || res.ColdStartMsP50 != nil {
		t.Errorf("memory/cold start = %v / %v", res.EstimatedMemoryFreedBytes, res.ColdStartMsP50)
	}
	found := false
	for _, p := range res.Settings.IgnorePaths {
		found = found || p == "/healthz"
	}
	if !found {
		t.Errorf("default ignore paths missing /healthz: %v", res.Settings.IgnorePaths)
	}

	rt.sleepPortWake = nil
	if res := sleepGet(t, rt, "web"); res.Eligible || res.BlockedReason == "" || res.Blockers[0].Code != appsleep.CodePinnedPort {
		t.Errorf("without port wake: %+v", res)
	}
}

func TestAppSleepPutStoresOptions(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if rec := sleepPut(t, rt.handleSetAppSleep, `{"idle_minutes":30,"ignore_paths":["/metrics"],"port_wake":false}`); rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	a, _ := db.GetAppSleep(context.Background(), "web")
	if len(a.IgnorePaths) != 1 || a.IgnorePaths[0] != "/metrics" || a.PortWake {
		t.Fatalf("stored = %+v", a)
	}
}

func TestAppsSleepOverview(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	port := 3000
	ctx := context.Background()
	for _, svc := range []store.DesiredService{
		{Name: "ok", Image: "nginx:1", Port: 80, Domains: []string{"ok.example.com"}},
		{Name: "pinned", Image: "nginx:1", Port: 80, HostPort: &port},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetAppSleepIdle(ctx, "pinned", 30, timeNow()); err != nil {
		t.Fatal(err)
	}
	fleet, err := rt.sleepFleet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := db.ListAppSleep(ctx)
	settings := map[string]store.AppSleep{}
	for _, r := range rows {
		settings[r.ServiceName] = r
	}
	out := rt.buildAppsSleepOverview(fleet, settings, func(string) bool { return true }, timeNow())
	names := map[string]sleepOverviewItem{}
	for _, a := range out.Apps {
		names[a.Name] = a
	}
	if !names["ok"].Eligible || names["ok"].Enabled {
		t.Errorf("ok = %+v", names["ok"])
	}
	if p := names["pinned"]; !p.Enabled || p.Eligible || !strings.Contains(strings.Join(p.BlockerCodes, ","), appsleep.CodePinnedPort) {
		t.Errorf("pinned = %+v", p)
	}
	if out.Enabled != 1 || out.Eligible < 1 {
		t.Errorf("counts = %+v", out)
	}
}

func timeNow() time.Time { return time.Now() }
