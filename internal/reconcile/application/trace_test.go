package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const traceAttempt = "dep_trace"

// fakeTrace is an in-memory TraceStore that resolves every image to one attempt.
type fakeTrace struct {
	mu     sync.Mutex
	steps  map[string]store.DeployTraceStep
	sealed bool
}

func newFakeTrace() *fakeTrace { return &fakeTrace{steps: map[string]store.DeployTraceStep{}} }

func (f *fakeTrace) SaveDeployTraceStep(_ context.Context, s store.DeployTraceStep) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps[s.Key] = s
	return nil
}

func (f *fakeTrace) TraceTargetFor(_ context.Context, _, _ string) (string, bool, error) {
	return traceAttempt, f.sealed, nil
}

func (f *fakeTrace) get(key string) (store.DeployTraceStep, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.steps[key]
	return s, ok
}

func (f *fakeTrace) must(t *testing.T, key string) store.DeployTraceStep {
	t.Helper()
	s, ok := f.get(key)
	if !ok {
		f.mu.Lock()
		defer f.mu.Unlock()
		keys := make([]string, 0, len(f.steps))
		for k := range f.steps {
			keys = append(keys, k)
		}
		t.Fatalf("trace has no step %q; have %v", key, keys)
	}
	return s
}

func (f *fakeTrace) waitFor(t *testing.T, key string) store.DeployTraceStep {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s, ok := f.get(key); ok {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	return f.must(t, key)
}

// traceRuntime scripts what the daemon reports during Create and afterwards.
type traceRuntime struct {
	*fakeRuntime
	pull      []docker.PullEvent
	pullErr   error
	details   *docker.ImageDetails
	node      docker.NodePlatform
	events    chan docker.LifecycleEvent
	eventsErr chan error
}

func (r *traceRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	for _, ev := range r.pull {
		if spec.OnPull != nil {
			spec.OnPull(ev)
		}
	}
	if r.pullErr != nil {
		return "", r.pullErr
	}
	return r.fakeRuntime.Create(ctx, spec)
}

func (r *traceRuntime) InspectImageDetails(context.Context, string) (*docker.ImageDetails, error) {
	return r.details, nil
}

func (r *traceRuntime) NodePlatform(context.Context) (docker.NodePlatform, error) {
	return r.node, nil
}

func (r *traceRuntime) WatchContainer(context.Context, string) (<-chan docker.LifecycleEvent, <-chan error) {
	return r.events, r.eventsErr
}

func newTraceRuntime(port int) *traceRuntime {
	return &traceRuntime{
		fakeRuntime: newFakeRuntime(port),
		details:     &docker.ImageDetails{ID: "sha256:abc", Arch: "amd64", OS: "linux", SizeBytes: 42 << 20, Entrypoint: []string{"/app"}, ExposedPorts: []string{"80/tcp"}},
		node:        docker.NodePlatform{Arch: "amd64", OS: "linux"},
		events:      make(chan docker.LifecycleEvent, 8),
		eventsErr:   make(chan error, 1),
	}
}

func pullScript() []docker.PullEvent {
	return []docker.PullEvent{
		{Phase: docker.PullPhaseStart, Ref: "img:v2"},
		{Phase: docker.PullPhaseProgress, Ref: "img:v2", Percent: 40, Layers: 3, LayersDone: 1, BytesCurrent: 4 << 20, BytesTotal: 10 << 20},
		{Phase: docker.PullPhaseLayer, LayerID: "l1", LayerState: docker.LayerComplete, LayerTotal: 1 << 20},
		{Phase: docker.PullPhaseDone, Ref: "img:v2", Layers: 3, LayersDone: 3, Percent: 100, Digest: "sha256:feed", Status: "Downloaded newer image for img:v2"},
	}
}

func traceDesired() *store.DesiredService {
	return &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green", Replicas: 1,
		Env:       map[string]string{"MODE": "prod"},
		SecretEnv: []store.SecretEnvRef{{Name: "API_KEY"}},
	}
}

func traceController(desired *store.DesiredService, rt docker.Runtime, tr *fakeTrace, opts ...Option) *Controller {
	opts = append([]Option{WithTraceStore(tr), WithSecretResolver(newFakeSecretResolver(map[string]string{"web/API_KEY": "s3cret-value"}))}, opts...)
	return New("web", &fakeStore{svc: desired}, rt, opts...)
}

func TestTrace_ImageDeploySteps(t *testing.T) {
	tr := newFakeTrace()
	rt := newTraceRuntime(0)
	rt.pull = pullScript()
	v1 := ContainerName("web", "img:v1", "")
	rt.seed(v1, true)
	c := traceController(traceDesired(), rt, tr)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	cfg := tr.must(t, deploytrace.StepConfigValidate)
	if cfg.Status != store.TraceStatusDone || !strings.Contains(cfg.Detail, "API_KEY (secret, masked)") || !strings.Contains(cfg.Detail, "MODE") {
		t.Errorf("config_validate = %+v, want env names with the secret masked", cfg)
	}
	for k, v := range tr.steps {
		if strings.Contains(v.Detail, "s3cret-value") || strings.Contains(v.Detail, "MODE=prod") {
			t.Errorf("step %q leaked an env value: %q", k, v.Detail)
		}
	}
	pull := tr.must(t, deploytrace.StepImagePull)
	if pull.Status != store.TraceStatusDone || !strings.Contains(pull.Detail, "sha256:feed") {
		t.Errorf("image_pull = %+v, want done with the digest", pull)
	}
	if l := tr.must(t, "image_pull_layer.l1"); l.Status != store.TraceStatusDone {
		t.Errorf("layer step = %+v, want done", l)
	}
	for _, key := range []string{deploytrace.StepContainerCreate, deploytrace.StepContainerStart, deploytrace.StepImageInspect, deploytrace.StepEnsureNetwork, deploytrace.StepCutover, deploytrace.StepSummary} {
		if s := tr.must(t, key); s.Status != store.TraceStatusDone {
			t.Errorf("step %q status = %q, want done (%+v)", key, s.Status, s)
		}
	}
	if insp := tr.must(t, deploytrace.StepImageInspect); !strings.Contains(insp.Detail, "matches node linux/amd64") {
		t.Errorf("image_inspect detail = %q, want the node match", insp.Detail)
	}
}

func TestTrace_CachedImageIsSkippedNotPulled(t *testing.T) {
	tr := newFakeTrace()
	rt := newTraceRuntime(0)
	rt.pull = []docker.PullEvent{{Phase: docker.PullPhaseDone, Ref: "img:v2", Status: docker.PullStatusCached, Percent: 100}}
	c := traceController(traceDesired(), rt, tr)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if s := tr.must(t, deploytrace.StepImagePull); s.Status != store.TraceStatusSkipped || s.Reason != "Cached" {
		t.Errorf("image_pull = %+v, want skipped/Cached", s)
	}
}

func TestTrace_FailuresNameTheStepAndKeepOldServing(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(rt *traceRuntime)
		failedStep string
		wantReason string
		wantText   string
	}{
		{
			name: "pull fails mid layer",
			setup: func(rt *traceRuntime) {
				rt.pull = pullScript()[:3]
				rt.pullErr = errors.New("docker: pull image \"img:v2\": unexpected EOF")
			},
			failedStep: deploytrace.StepImagePull, wantReason: "ImagePullFailed", wantText: "unexpected EOF",
		},
		{
			name: "registry auth denied",
			setup: func(rt *traceRuntime) {
				rt.pull = pullScript()[:1]
				rt.pullErr = errors.New("docker: pull image \"img:v2\": pull access denied, repository does not exist or may require authorization: unauthorized")
			},
			failedStep: deploytrace.StepImagePull, wantReason: "RegistryAuthDenied", wantText: "unauthorized",
		},
		{
			name: "create fails after a good pull",
			setup: func(rt *traceRuntime) {
				rt.pull = pullScript()
				rt.createErr = errors.New("Conflict. The container name is already in use")
			},
			failedStep: deploytrace.StepContainerCreate, wantReason: "CreateFailed", wantText: "already in use",
		},
		{
			name: "start fails",
			setup: func(rt *traceRuntime) {
				rt.pull = pullScript()
				rt.startErr = errors.New("driver failed programming external connectivity")
			},
			failedStep: deploytrace.StepContainerStart, wantReason: "StartFailed", wantText: "external connectivity",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newFakeTrace()
			rt := newTraceRuntime(0)
			v1 := ContainerName("web", "img:v1", "")
			rt.seed(v1, true)
			tt.setup(rt)
			c := traceController(traceDesired(), rt, tr)

			if _, err := c.Reconcile(context.Background()); err == nil {
				t.Fatal("Reconcile() error = nil, want the failure surfaced")
			}
			step := tr.must(t, tt.failedStep)
			if step.Status != store.TraceStatusFailed || step.Reason != tt.wantReason || !strings.Contains(step.Detail, tt.wantText) {
				t.Errorf("%s = %+v, want failed/%s carrying %q", tt.failedStep, step, tt.wantReason, tt.wantText)
			}
			cut := tr.must(t, deploytrace.StepCutover)
			if cut.Status != store.TraceStatusSkipped || !strings.Contains(cut.Detail, v1+" keeps serving") {
				t.Errorf("cutover = %+v, want skipped and naming %s as still serving", cut, v1)
			}
			sum := tr.must(t, deploytrace.StepSummary)
			if sum.Status != store.TraceStatusFailed || !strings.Contains(sum.Detail, tt.wantText) {
				t.Errorf("summary = %+v, want failed with the Docker text", sum)
			}
			if cs := rt.containers[v1]; cs == nil || !cs.Running {
				t.Fatal("old container not serving after the failed deploy")
			}
		})
	}
	t.Run("auth denial also fails the registry auth step", func(t *testing.T) {
		tr := newFakeTrace()
		rt := newTraceRuntime(0)
		rt.pull = pullScript()[:1]
		rt.pullErr = errors.New("unauthorized: authentication required")
		c := traceController(traceDesired(), rt, tr)
		_, _ = c.Reconcile(context.Background())
		if s := tr.must(t, deploytrace.StepRegistryAuth); s.Status != store.TraceStatusFailed || s.Reason != "RegistryAuthDenied" {
			t.Errorf("registry_auth = %+v, want failed/RegistryAuthDenied", s)
		}
	})
}

func TestTrace_ArchMismatchWarns(t *testing.T) {
	tr := newFakeTrace()
	rt := newTraceRuntime(0)
	rt.pull = pullScript()
	rt.details.Arch = "arm64"
	c := traceController(traceDesired(), rt, tr)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	s := tr.must(t, deploytrace.StepImageInspect)
	if s.Status != store.TraceStatusWarning || s.Reason != "PlatformMismatch" || !strings.Contains(s.Detail, "arm64") {
		t.Errorf("image_inspect = %+v, want a PlatformMismatch warning", s)
	}
}

func TestTrace_HealthNeverReady_ProbesRecordedOldKeepsServing(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	tr := newFakeTrace()
	rt := newTraceRuntime(serverPort(t, srv))
	rt.pull = pullScript()
	v1 := ContainerName("web", "img:v1", "")
	rt.seed(v1, true)
	desired := traceDesired()
	desired.Health = &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}}
	c := traceController(desired, rt, tr, WithReadyBudget(150*time.Millisecond))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want readiness failure")
	}
	probe := tr.must(t, "readiness_probe.1")
	if probe.Status != store.TraceStatusWarning || !strings.Contains(probe.Detail, "status 503") {
		t.Errorf("probe = %+v, want a warning with status 503", probe)
	}
	gate := tr.must(t, deploytrace.StepReadinessGate)
	if gate.Status != store.TraceStatusFailed {
		t.Errorf("readiness_gate = %+v, want failed", gate)
	}
	if cut := tr.must(t, deploytrace.StepCutover); cut.Status != store.TraceStatusSkipped || !strings.Contains(cut.Detail, "keeps serving") {
		t.Errorf("cutover = %+v, want skipped with the old container serving", cut)
	}
	if cs := rt.containers[v1]; cs == nil || !cs.Running {
		t.Fatal("old container not serving")
	}
}

func TestTrace_OOMDuringReadiness_DaemonEventsExplainIt(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	tr := newFakeTrace()
	rt := newTraceRuntime(serverPort(t, srv))
	rt.pull = pullScript()
	v1 := ContainerName("web", "img:v1", "")
	v2 := ContainerName("web", "img:v2", "")
	rt.seed(v1, true)
	desired := traceDesired()
	desired.Health = &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}}
	rt.events <- docker.LifecycleEvent{Action: "start"}
	rt.events <- docker.LifecycleEvent{Action: "die", ExitCode: 137, HasExit: true, OOMKilled: true}
	time.AfterFunc(40*time.Millisecond, func() { rt.crashContainer(v2, true, 137) })
	c := traceController(desired, rt, tr, WithReadyBudget(2*time.Second))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want the OOM kill surfaced")
	}
	die := tr.waitFor(t, "docker_event.die")
	if die.Status != store.TraceStatusFailed || die.Reason != "OOMKilled" || !strings.Contains(die.Detail, "code 137") {
		t.Errorf("docker die event = %+v, want failed/OOMKilled with exit code 137", die)
	}
	if start := tr.waitFor(t, "docker_event.start"); start.Status != store.TraceStatusDone {
		t.Errorf("docker start event = %+v, want done", start)
	}
	sum := tr.must(t, deploytrace.StepSummary)
	if sum.Status != store.TraceStatusFailed {
		t.Errorf("summary = %+v, want failed", sum)
	}
	if cs := rt.containers[v1]; cs == nil || !cs.Running {
		t.Fatal("old container not serving after the OOM kill")
	}
}

func TestTrace_EventStreamDisconnect_DeploySucceedsAndTraceSaysSo(t *testing.T) {
	tr := newFakeTrace()
	rt := newTraceRuntime(0)
	rt.pull = pullScript()
	rt.eventsErr <- errors.New("EOF")
	c := traceController(traceDesired(), rt, tr)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v, want the deploy unaffected by a lost event stream", err)
	}
	lost := tr.waitFor(t, "docker_event.stream")
	if lost.Status != store.TraceStatusWarning || lost.Reason != "EventStreamLost" {
		t.Errorf("event stream step = %+v, want a warning", lost)
	}
	if s := tr.must(t, deploytrace.StepSummary); s.Status != store.TraceStatusDone {
		t.Errorf("summary = %+v, want done", s)
	}
}

func TestTrace_SealedAttemptIsNotRewritten(t *testing.T) {
	tr := newFakeTrace()
	tr.sealed = true
	rt := newTraceRuntime(0)
	rt.pull = pullScript()
	c := traceController(traceDesired(), rt, tr)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(tr.steps) != 0 {
		t.Errorf("sealed attempt gained %d steps, want none", len(tr.steps))
	}
}
