package ingress

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestController_Reconcile_HoldsHostWhenBackendDisappears(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")
	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	tracker := ingress.NewHoldTracker(time.Minute)
	c := New(st, rt, applier, WithLogger(discardLogger()), WithHoldTracker(tracker), WithHardening(ingress.DefaultHardening()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if routes := applier.routes(t); len(routes) != 2 {
		t.Fatalf("running: routes = %d, want 2", len(routes))
	}

	rt.mu.Lock()
	delete(rt.containers, target)
	rt.mu.Unlock()

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	routes := applier.routes(t)
	if len(routes) != 2 {
		t.Fatalf("backend gone: routes = %d, want hold + diagnostic routes", len(routes))
	}
	h, ok := pageHandler(routes[0].Handle[0])
	if !ok || h.StatusCode != 503 || h.Headers["Retry-After"] == nil {
		t.Errorf("hold handler = %+v", routes[0].Handle[0])
	}
	if msg := conditionOf(t, result).Message; msg == "" {
		t.Error("condition message empty")
	}
}

func TestController_Reconcile_NeverRoutedHostIsNotHeld(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"new.example.com"}}
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, newFakeRuntime(), applier, WithLogger(discardLogger()), WithHoldTracker(ingress.NewHoldTracker(time.Minute)))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if routes := applier.routes(t); len(routes) != 0 {
		t.Errorf("a never-deployed host must not trigger certificate issuance, got routes %+v", routes)
	}
}

func holdPageFor(t *testing.T, desired store.DesiredService, attempts []store.DeployAttempt, show func(context.Context) bool) *httptest.ResponseRecorder {
	t.Helper()
	target := application.ContainerName(desired.Name, desired.Image, "")
	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	opts := []Option{WithLogger(discardLogger()), WithHoldTracker(ingress.NewHoldTracker(time.Minute)), WithHardening(ingress.DefaultHardening())}
	if show != nil {
		opts = append(opts, WithHoldReasonGate(show))
	}
	c := New(st, rt, applier, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	delete(rt.containers, target)
	rt.mu.Unlock()
	st.attempts = map[string][]store.DeployAttempt{desired.Name: attempts}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	h, ok := pageHandler(applier.routes(t)[0].Handle[0])
	if !ok {
		t.Fatal("no hold handler")
	}
	rec := httptest.NewRecorder()
	for k, v := range h.Headers {
		rec.Header()[k] = v
	}
	rec.WriteHeader(h.StatusCode)
	_, _ = rec.WriteString(h.Body)
	return rec
}

func TestController_HoldPageReasons(t *testing.T) {
	on := func(context.Context) bool { return true }
	base := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Replicas: 1, Domains: []string{"app.example.com"}}
	stopped := base
	stopped.Suspended = true
	zero := base
	zero.Replicas = 0
	running := []store.DeployAttempt{{ServiceName: "web", Status: store.DeployAttemptStatusRunning}}
	tests := []struct {
		name     string
		svc      store.DesiredService
		attempts []store.DeployAttempt
		show     func(context.Context) bool
		want     string
		refresh  bool
	}{
		{"suspended is stopped", stopped, nil, on, "This app is stopped.", false},
		{"zero replicas is stopped", zero, nil, on, "This app is stopped.", false},
		{"deploying", base, running, on, "being deployed", true},
		{"unhealthy", base, nil, on, "not responding", true},
		{"show state off stays generic", stopped, nil, nil, "being updated or was just stopped", true},
		{"gate false stays generic", base, running, func(context.Context) bool { return false }, "being updated or was just stopped", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := holdPageFor(t, tt.svc, tt.attempts, tt.show)
			if rec.Code != 503 {
				t.Fatalf("status = %d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.want) {
				t.Errorf("body missing %q: %s", tt.want, body)
			}
			if got := strings.Contains(body, `http-equiv="refresh"`); got != tt.refresh {
				t.Errorf("refresh = %v, want %v", got, tt.refresh)
			}
			if got := rec.Header().Get("Retry-After") != ""; got != tt.refresh {
				t.Errorf("Retry-After present = %v, want %v", got, tt.refresh)
			}
			if strings.Contains(body, "web") {
				t.Error("page leaked the app name")
			}
		})
	}
}
