package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deployurls"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A rollback that lands while the newer release is in its readiness wait ends
// the wait early, records nothing against the old release and leaves the
// next pass to reconcile the new desired state.
func TestAwaitReady_SupersededByRollback(t *testing.T) {
	rt, st, _, _ := reviveFixture(t, true)
	st.afterFirst = &store.DesiredService{Name: "web", Image: "app:v2", Port: 80}
	rollouts := &fakeRollouts{}
	gates := &fakeGateRecorder{}
	c := New("web", st, rt, WithReadyBudget(20*time.Second), WithReviveWatch(30*time.Millisecond),
		WithRolloutRecorder(rollouts), WithGateRecorder(gates),
		WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
	started := time.Now()
	res, err := c.Reconcile(context.Background())
	if took := time.Since(started); took > 5*time.Second {
		t.Fatalf("pass took %s, the wait was not interrupted", took)
	}
	if err != nil || len(res.Conditions) != 0 {
		t.Fatalf("superseded pass must publish nothing, got %+v, %v", res, err)
	}
	rollouts.mu.Lock()
	failed := len(rollouts.failed)
	rollouts.mu.Unlock()
	if failed != 0 || gates.has("readiness", "failed") {
		t.Fatalf("a superseded wait must not record a failure (rollout failures %d)", failed)
	}
	newName := ContainerName("web", NameImage(store.DesiredService{Name: "web", Image: "app:v3", Port: 80}), "")
	if running(rt, newName) {
		t.Error("the aborted candidate must be stopped")
	}
	// The next pass starts from the new desired state with no leftover delay.
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("next pass: %v", err)
	}
}

func TestDesiredChanged(t *testing.T) {
	base := &store.DesiredService{Image: "a", RestartNonce: "1", Replicas: 1}
	for name, mut := range map[string]func(*store.DesiredService){
		"image": func(s *store.DesiredService) { s.Image = "b" }, "nonce": func(s *store.DesiredService) { s.RestartNonce = "2" },
		"replicas": func(s *store.DesiredService) { s.Replicas = 2 }, "suspended": func(s *store.DesiredService) { s.Suspended = true },
	} {
		n := *base
		mut(&n)
		if !desiredChanged(base, &n) {
			t.Errorf("%s change not detected", name)
		}
	}
	same := *base
	if desiredChanged(base, &same) {
		t.Error("identical desired reported changed")
	}
}

type fakeRetained struct {
	mu      sync.Mutex
	stopAt  int
	unready int
	ready   int
}

func (f *fakeRetained) NoteUnready(context.Context, string) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unready++
	return true, f.unready >= f.stopAt, nil
}
func (f *fakeRetained) NoteReady(string)                        { f.mu.Lock(); f.ready++; f.mu.Unlock() }
func (f *fakeRetained) IsRetained(context.Context, string) bool { return true }

// A retained deploy URL that keeps failing readiness is reported as stopped
// once the limit is reached, not as an ordinary failure to retry.
func TestRetainedDeploy_StopsAfterLimit(t *testing.T) {
	rt, st, _, _ := reviveFixture(t, true)
	f := &fakeRetained{stopAt: 2}
	c := New("web", st, rt, WithReadyBudget(300*time.Millisecond), WithRetainedDeploys(f),
		WithRestartBackoff(NewRestartBackoff(time.Millisecond, time.Millisecond, time.Hour)))
	res, _ := c.Reconcile(context.Background())
	if len(res.Conditions) == 0 || res.Conditions[0].Reason == reasonRetainedDeployStopped {
		t.Fatalf("first failure must not stop the service: %+v", res.Conditions)
	}
	res, _ = c.Reconcile(context.Background())
	if len(res.Conditions) == 0 || res.Conditions[0].Reason != reasonRetainedDeployStopped {
		t.Fatalf("limit reached must surface %s, got %+v", reasonRetainedDeployStopped, res.Conditions)
	}
}

// A retained service gets its app's secret env, so it becomes ready.
func TestRetainedDeploy_ResolvesAppSecrets(t *testing.T) {
	desired := &store.DesiredService{Name: "web-abc1234", Image: "app:v2", Port: 80, SecretEnv: []store.SecretEnvRef{{Name: "WHOAMI_PORT_NUMBER", Required: true}}}
	rt := newFakeRuntime(0)
	apps := fakeApps{"web-abc1234": "web"}
	res := deployurls.ParentSecrets{Inner: newFakeSecretResolver(map[string]string{"web/WHOAMI_PORT_NUMBER": "80"}), Apps: apps}
	c := New("web-abc1234", &fakeStore{svc: desired}, rt, WithSecretResolver(res))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := rt.lastCreateEnv["WHOAMI_PORT_NUMBER"]; got != "80" {
		t.Errorf("retained service env = %q, want the app's secret", got)
	}
}

type fakeApps map[string]string

func (f fakeApps) DeployURLApp(_ context.Context, name string) (string, bool, error) {
	a, ok := f[name]
	return a, ok, nil
}
