package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func exposureDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) (*httptest.ResponseRecorder, exposureResource) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	var res exposureResource
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	return rec, res
}

func TestAppExposure(t *testing.T) {
	prev := hostListeningPorts
	hostListeningPorts = func() []int { return []int{25000} }
	t.Cleanup(func() { hostListeningPorts = prev })

	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	other := 24000
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "i:1", Port: 3000},
		{Name: "api", Image: "i:1", Port: 3000, HostPort: &other, BindAddress: "public"},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	const path = "/api/v1/apps/web/exposure"

	t.Run("validation and conflicts", func(t *testing.T) {
		cases := []struct {
			name, body string
			status     int
			wantMsg    string
		}{
			{"bad mode", `{"mode":"open"}`, 400, "mode must be"},
			{"zero container port", `{"container_port":0}`, 400, "container_port"},
			{"host port too high", `{"host_port":70000}`, 400, "host_port"},
			{"control plane port", `{"host_port":8080}`, 409, "control plane"},
			{"other app port", `{"host_port":24000}`, 409, "app api"},
			{"foreign listener", `{"host_port":25000}`, 409, "another process"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				rec, _ := exposureDo(t, rt, cookie, http.MethodPost, path+"/preview", c.body)
				if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.wantMsg) {
					t.Fatalf("got %d %s", rec.Code, rec.Body.String())
				}
			})
		}
	})

	t.Run("preview does not write", func(t *testing.T) {
		rec, _ := exposureDo(t, rt, cookie, http.MethodPost, path+"/preview", `{"mode":"direct","host_port":24100}`)
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		svc, _ := db.GetDesiredService(ctx, "web")
		if svc.HostPort != nil {
			t.Fatal("preview pinned a port")
		}
	})

	t.Run("direct then proxied", func(t *testing.T) {
		rec, res := exposureDo(t, rt, cookie, http.MethodPut, path, `{"mode":"direct","host_port":24100,"container_port":8000}`)
		if rec.Code != 200 || res.Mode != exposureModeDirect || res.HostPort != 24100 || res.ContainerPort != 8000 || res.FirewallRow == "" {
			t.Fatalf("direct: %d %s", rec.Code, rec.Body.String())
		}
		svc, _ := db.GetDesiredService(ctx, "web")
		if svc.Port != 8000 || svc.HostPort == nil || *svc.HostPort != 24100 || svc.BindAddress != "public" {
			t.Fatalf("saved %+v", svc)
		}
		rule, ok := rt.exposureRule(ctx, "web")
		if !ok || rule.Port != 24100 {
			t.Fatalf("rule %+v ok=%v", rule, ok)
		}

		rec, res = exposureDo(t, rt, cookie, http.MethodPut, path, `{"mode":"proxied"}`)
		if rec.Code != 200 || res.Mode != exposureModeProxied || res.FirewallRow != "" {
			t.Fatalf("proxied: %d %s", rec.Code, rec.Body.String())
		}
		svc, _ = db.GetDesiredService(ctx, "web")
		if svc.HostPort != nil || svc.BindAddress != "private" {
			t.Fatalf("not reverted: %+v", svc)
		}
	})

	t.Run("auto pick lands in range", func(t *testing.T) {
		_, res := exposureDo(t, rt, cookie, http.MethodPut, path, `{"mode":"direct"}`)
		if res.HostPort < 20000 || res.HostPort > 29999 || res.HostPort == 25000 || res.HostPort == 24000 {
			t.Fatalf("picked %d", res.HostPort)
		}
		_, got := exposureDo(t, rt, cookie, http.MethodGet, path, "")
		if got.Mode != exposureModeDirect || !got.LocalNode {
			t.Fatalf("%+v", got)
		}
		var sawAPI bool
		for _, h := range got.PortsInUse {
			if h.Port == 24000 && h.Holder == "api" {
				sawAPI = true
			}
			if h.Port == res.HostPort {
				t.Fatalf("own port listed as in use: %+v", h)
			}
		}
		if !sawAPI {
			t.Fatalf("api port missing from %+v", got.PortsInUse)
		}
	})
}

func TestAppExposure_UnknownApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec, _ := exposureDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/ghost/exposure", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAppExposureCheck_NotRunning(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "i:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exposure/check", ""))
	var got exposureCheckResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || got.Scope != reachabilityScopeCPHost || len(got.Targets) != 1 || got.Targets[0].Failure != probeContainerNotRunning {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
