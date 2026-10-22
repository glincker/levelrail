package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type memProvisionStore struct {
	events []store.DatabaseProvisionEvent
}

func (m *memProvisionStore) AppendDatabaseProvisionEvent(_ context.Context, e store.DatabaseProvisionEvent) error {
	e.ID = int64(len(m.events) + 1)
	m.events = append(m.events, e)
	return nil
}

func (m *memProvisionStore) BeginDatabaseProvisionAttempt(context.Context, string, int) (string, error) {
	return "prov_test", nil
}

func (m *memProvisionStore) LatestDatabaseProvisionAttempt(context.Context, string) (string, []store.DatabaseProvisionEvent, error) {
	if len(m.events) == 0 {
		return "", nil, nil
	}
	return "prov_test", m.events, nil
}

// pullingRuntime reports pull progress from Create the way the real client does.
type pullingRuntime struct {
	*fakeRuntime
	pullErr bool
}

func (p *pullingRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	if spec.OnPull != nil {
		spec.OnPull(docker.PullEvent{Phase: docker.PullPhaseStart, Ref: spec.Image})
		spec.OnPull(docker.PullEvent{Phase: docker.PullPhaseProgress, Layers: 4, LayersDone: 1, Percent: 25})
		if p.pullErr {
			return "", errors.New("docker: pull image: dial tcp: no such host")
		}
		spec.OnPull(docker.PullEvent{Phase: docker.PullPhaseProgress, Layers: 4, LayersDone: 4, Percent: 100})
		spec.OnPull(docker.PullEvent{Phase: docker.PullPhaseDone, Percent: 100})
	}
	return p.fakeRuntime.Create(ctx, spec)
}

func provisionFixture(t *testing.T) (*memProvisionStore, *dbprovision.Recorder) {
	t.Helper()
	t.Setenv(dbprovision.EnvEventInterval, "0s")
	ms := &memProvisionStore{}
	rec := dbprovision.NewRecorder(ms)
	if _, err := rec.Begin(context.Background(), "main"); err != nil {
		t.Fatal(err)
	}
	return ms, rec
}

func statusOf(a dbprovision.Attempt, step string) string {
	for _, s := range a.Steps {
		if s.Step == step {
			return s.Status
		}
	}
	return ""
}

func TestProvision_SuccessEmitsStepsInOrderWithPullProgress(t *testing.T) {
	ms, rec := provisionFixture(t)
	rt := &pullingRuntime{fakeRuntime: newFakeRuntime()}
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	c := New("main", &fakeStore{db: desired}, rt, WithProvisionRecorder(rec))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	a := dbprovision.View("prov_test", ms.events)
	if a.Status != dbprovision.AttemptReady {
		t.Fatalf("status = %s, want ready; steps = %+v", a.Status, a.Steps)
	}
	var order []string
	seen := map[string]bool{}
	var maxPct int64
	for _, e := range ms.events {
		if !seen[e.Step] {
			seen[e.Step] = true
			order = append(order, e.Step)
		}
		if e.Step == dbprovision.StepPullImage && e.Current > maxPct {
			maxPct = e.Current
		}
	}
	idx := func(s string) int {
		for i, v := range order {
			if v == s {
				return i
			}
		}
		return -1
	}
	for _, pair := range [][2]string{{"accepted", "docker_check"}, {"create_volume", "create_container"}, {"create_container", "start"}, {"start", "wait_ready"}, {"wait_ready", "ready"}} {
		if idx(pair[0]) < 0 || idx(pair[0]) > idx(pair[1]) {
			t.Errorf("%s must come before %s, order = %v", pair[0], pair[1], order)
		}
	}
	if maxPct != 100 {
		t.Errorf("pull percent reached %d, want 100", maxPct)
	}
}

func TestProvision_PullFailureSkipsLaterSteps(t *testing.T) {
	ms, rec := provisionFixture(t)
	rt := &pullingRuntime{fakeRuntime: newFakeRuntime(), pullErr: true}
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	c := New("main", &fakeStore{db: desired}, rt, WithProvisionRecorder(rec))
	_, _ = c.Reconcile(context.Background())
	a := dbprovision.View("prov_test", ms.events)
	if a.Status != dbprovision.AttemptFailed || a.Failure == nil || a.Failure.Step != dbprovision.StepPullImage {
		t.Fatalf("attempt = %+v, want failed at pull_image", a)
	}
	if a.Failure.Fix == nil || a.Failure.Fix.ID != dbprovision.FixRetry {
		t.Errorf("fix = %+v, want retry", a.Failure.Fix)
	}
	for _, s := range []string{"create_container", "start", "wait_ready", "apply_settings", "ready"} {
		if got := statusOf(a, s); got != dbprovision.StatusSkipped {
			t.Errorf("step %s = %s, want skipped", s, got)
		}
	}
}

func TestProvision_FailureAtEachReason(t *testing.T) {
	cases := []struct {
		reason, errText, wantStep string
		pullDone                  bool
	}{
		{"CredentialsNotConfigured", "no credentials", "credentials_and_tls", true},
		{"VolumeFailed", "disk", "create_volume", true},
		{"VolumeFailed", "dial unix /run/docker-guard.sock: timeout", "docker_check", true},
		{"InspectFailed", "x", "docker_check", true},
		{"CreateFailed", "create: boom", "create_container", true},
		{"CreateFailed", "pull image: x", "pull_image", false},
		{"StartFailedAfterCreate", "x", "start", true},
		{"VanishedAfterStart", "x", "wait_ready", true},
	}
	for _, tc := range cases {
		step := dbprovision.FailStepForReason(tc.reason, tc.pullDone, tc.errText)
		if step != tc.wantStep {
			t.Errorf("%s/%q: step = %s, want %s", tc.reason, tc.errText, step, tc.wantStep)
		}
		ms := &memProvisionStore{}
		rec := dbprovision.NewRecorder(ms)
		rec.SetClock(func() time.Time { return time.Unix(1, 0) })
		tr, _ := rec.Begin(context.Background(), "main")
		tr.Fail(step, errors.New(tc.errText))
		a := dbprovision.View("prov_test", ms.events)
		if statusOf(a, step) != dbprovision.StatusFailed || statusOf(a, "ready") != dbprovision.StatusSkipped {
			t.Errorf("%s: failure not marked, steps = %+v", tc.reason, a.Steps)
		}
	}
}
