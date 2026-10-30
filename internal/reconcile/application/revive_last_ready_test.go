package application

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type historyStore struct {
	fakeStore
	attempts []store.DeployAttempt
}

func (h *historyStore) ListDeployAttempts(context.Context, string) ([]store.DeployAttempt, error) {
	return h.attempts, nil
}

func TestReviveLastReady(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "app:v3", Port: 80}
	oldSvc := store.DesiredService{Name: "web", Image: "app:v2", Port: 80}
	failedSvc := store.DesiredService{Name: "web", Image: "app:v1", Port: 80}
	oldName := ContainerName("web", NameImage(oldSvc), "")
	failedName := ContainerName("web", NameImage(failedSvc), "")

	tests := []struct {
		name        string
		oldRunning  bool
		wantRestart bool
	}{
		{"live container dead, desired failed", false, true},
		{"live container still running", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := newFakeRuntime(8080)
			rt.createErr = errors.New("boom")
			rt.seed(oldName, tc.oldRunning)
			rt.containers[oldName].Image = "app:v2"
			rt.seed(failedName, false)
			rt.containers[failedName].Image = "app:v1"
			st := &historyStore{
				fakeStore: fakeStore{svc: desired},
				attempts: []store.DeployAttempt{
					{ID: "d3", Image: "app:v3", Status: store.DeployAttemptStatusFailed},
					{ID: "d2", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded},
					{ID: "d1", Image: "app:v1", Status: store.DeployAttemptStatusFailed},
				},
			}
			c := New("web", st, rt, WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
			_, _ = c.Reconcile(context.Background())
			if got := rt.containers[oldName].Running; got != true && tc.wantRestart {
				t.Errorf("last ready release not restarted")
			}
			if cs := rt.containers[failedName]; cs != nil && cs.Running {
				t.Errorf("a failed release must never be revived")
			}
			// Half-succeeded: a second pass inside the backoff must not restart again.
			rt.mu.Lock()
			rt.containers[oldName].Running = false
			rt.mu.Unlock()
			before := rt.startCalls
			_, _ = c.Reconcile(context.Background())
			if tc.wantRestart && rt.startCalls != before {
				t.Errorf("restarted again inside backoff")
			}
		})
	}
}

// Podman reports docker.io/ qualified image names; attempts keep the short ref.
func TestReviveLastReady_PodmanQualifiedImageNames(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "traefik/whoami:v2", Port: 80}
	oldSvc := store.DesiredService{Name: "web", Image: "traefik/whoami:v1", Port: 80}
	oldName := ContainerName("web", NameImage(oldSvc), "")
	rt := newFakeRuntime(8080)
	rt.createErr = errors.New("boom")
	rt.seed(oldName, false)
	rt.containers[oldName].Image = "docker.io/traefik/whoami:v1"
	st := &historyStore{
		fakeStore: fakeStore{svc: desired},
		attempts: []store.DeployAttempt{
			{ID: "d2", Image: "traefik/whoami:v2", Status: store.DeployAttemptStatusFailed},
			{ID: "d1", Image: "traefik/whoami:v1", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	c := New("web", st, rt, WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
	_, _ = c.Reconcile(context.Background())
	if !rt.containers[oldName].Running {
		t.Error("dead live container with a qualified image name was not revived")
	}
}

// A desired release that is running but never became ready must not stop the
// last ready release from coming back after its container was killed.
func TestReviveLastReady_RunningUnreadyDesiredDoesNotVeto(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "app:v3", Port: 80}
	oldSvc := store.DesiredService{Name: "web", Image: "app:v2", Port: 80}
	oldName := ContainerName("web", NameImage(oldSvc), "")
	newName := ContainerName("web", NameImage(*desired), "")
	rt := newFakeRuntime(8080)
	rt.seed(oldName, false)
	rt.containers[oldName].Image = "app:v2"
	rt.seed(newName, true)
	rt.containers[newName].Image = "app:v3"
	st := &historyStore{
		fakeStore: fakeStore{svc: desired},
		attempts: []store.DeployAttempt{
			{ID: "d3", Image: "app:v3", Status: store.DeployAttemptStatusFailed},
			{ID: "d2", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	c := New("web", st, rt, WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
	c.reviveLastReady(context.Background(), []string{newName}, desired)
	if !rt.containers[oldName].Running {
		t.Error("last ready release was not revived while the failing desired release runs")
	}
}

func reviveFixture(t *testing.T, oldRunning bool) (*fakeRuntime, *historyStore, *store.DesiredService, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	desired := &store.DesiredService{Name: "web", Image: "app:v3", Port: 80, Health: &store.ServiceHealth{
		Readiness: &store.ServiceProbe{Path: "/", Interval: 20 * time.Millisecond, Timeout: 100 * time.Millisecond},
	}}
	oldSvc := store.DesiredService{Name: "web", Image: "app:v2", Port: 80}
	oldName := ContainerName("web", NameImage(oldSvc), "")
	rt := newFakeRuntime(serverPort(t, srv))
	rt.seed(oldName, oldRunning)
	rt.containers[oldName].Image = "app:v2"
	st := &historyStore{
		fakeStore: fakeStore{svc: desired},
		attempts: []store.DeployAttempt{
			{ID: "d3", Image: "app:v3", Status: store.DeployAttemptStatusFailed},
			{ID: "d2", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	return rt, st, desired, oldName
}

func running(rt *fakeRuntime, name string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	cs := rt.containers[name]
	return cs != nil && cs.Running
}

func waitUntil(d time.Duration, cond func() bool) bool {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// A dead serving release comes back before the failing desired release's
// readiness wait ends, not after it.
func TestReviveLastReady_DoesNotWaitForFailingReadiness(t *testing.T) {
	// The budget only needs to outlast the revive; the pass then ends at it.
	for _, tc := range []struct {
		name       string
		oldRunning bool
	}{{"dead before the pass", false}, {"killed during the wait", true}} {
		t.Run(tc.name, func(t *testing.T) {
			rt, st, _, oldName := reviveFixture(t, tc.oldRunning)
			c := New("web", st, rt, WithReadyBudget(1*time.Second), WithReviveWatch(50*time.Millisecond),
				WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
			done := make(chan struct{})
			go func() { defer close(done); _, _ = c.Reconcile(context.Background()) }()
			if tc.oldRunning {
				time.Sleep(200 * time.Millisecond)
				rt.mu.Lock()
				rt.containers[oldName].Running = false
				rt.mu.Unlock()
			}
			started := time.Now()
			if !waitUntil(time.Second, func() bool { return running(rt, oldName) }) {
				t.Fatal("last ready release was not revived while the desired release waited for readiness")
			}
			select {
			case <-done:
				t.Fatalf("pass ended before the check, revive took %s", time.Since(started))
			default:
			}
			<-done
		})
	}
}

// With a pinned host port, a container mid-create on the port blocks the revive.
func TestReviveLastReady_PinnedPortHeldByCandidate(t *testing.T) {
	rt, st, desired, oldName := reviveFixture(t, false)
	hp := 8080
	desired.HostPort = &hp
	candidate := ContainerName("web", NameImage(*desired), "")
	rt.seed(candidate, true)
	rt.containers[candidate].Image = "app:v3"
	rt.containers[candidate].Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: hp}}
	c := New("web", st, rt, WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
	c.reviveLastReady(context.Background(), []string{candidate}, desired)
	if running(rt, oldName) {
		t.Error("revive started a release onto a host port another container is creating on")
	}
}

// A healthy redeploy keeps the old release running for the hold, never
// restarts anything meanwhile, and removes it (no exited leftover) afterwards.
func TestReconcile_RedeployHoldsPreviousThenRemoves(t *testing.T) {
	rt := newDigestRuntime(map[string]string{})
	oldName := ContainerName("web", "img:v1", "")
	rt.seedRunning(oldName, "", time.Now().Add(-time.Hour))
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80}
	st := &historyStore{
		fakeStore: fakeStore{svc: desired},
		attempts: []store.DeployAttempt{
			{ID: "d2", Image: "img:v2", Status: store.DeployAttemptStatusSucceeded},
			{ID: "d1", Image: "img:v1", Status: store.DeployAttemptStatusSucceeded},
		},
	}
	c := New("web", st, rt, WithPreviousReleaseHold(3*time.Minute), WithDrainTracker(NewDrainTracker()),
		WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
	for i := 0; i < 3; i++ {
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !running(rt.fakeRuntime, oldName) {
			t.Fatalf("pass %d: previous release stopped inside the hold", i)
		}
	}
	if rt.startCalls != 1 {
		t.Errorf("start calls = %d, want only the new release's own start", rt.startCalls)
	}
	rt.mu.Lock()
	for _, cs := range rt.containers {
		cs.Created = cs.Created.Add(-time.Hour)
	}
	rt.mu.Unlock()
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.count() != 1 {
		t.Fatalf("containers after the hold = %v, want only the new release", rt.names())
	}
}

func TestReviveLastReady_PublishedUDPPortHeldByCandidate(t *testing.T) {
	rt, st, desired, oldName := reviveFixture(t, false)
	desired.PublishedPorts = []store.PublishedPort{{ContainerPort: 53, HostPort: 5353, Protocol: store.PortProtocolUDP}}
	candidate := ContainerName("web", NameImage(*desired), "")
	rt.seed(candidate, true)
	rt.containers[candidate].Image = "app:v3"
	rt.containers[candidate].Ports = []docker.PortBinding{{ContainerPort: 53, HostPort: 5353, Protocol: store.PortProtocolUDP}}
	c := New("web", st, rt, WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
	c.reviveLastReady(context.Background(), []string{candidate}, desired)
	if running(rt, oldName) {
		t.Error("revive started a release onto a udp host port another container holds")
	}
}

func TestPrimaryAddrSkipsUDPBinding(t *testing.T) {
	state := &docker.ContainerState{Name: "web", Ports: []docker.PortBinding{{ContainerPort: 53, HostPort: 53, Protocol: "udp"}, {ContainerPort: 80, HostPort: 32768, Protocol: "tcp"}}}
	if got, err := PrimaryAddr(state, 80); err != nil || got != "127.0.0.1:32768" {
		t.Fatalf("PrimaryAddr = %q %v, want the tcp binding", got, err)
	}
}
