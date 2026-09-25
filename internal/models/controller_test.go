package models

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	model     *store.Model
	err       error
	endpoint  string
	deleted   bool
	deleteErr error
}

func (f *fakeStore) GetModel(context.Context, string) (*store.Model, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.model == nil {
		return nil, store.ErrModelNotFound
	}
	cp := *f.model
	return &cp, nil
}

func (f *fakeStore) SetModelEndpoint(_ context.Context, _, dial string) error {
	f.endpoint = dial
	if f.model != nil {
		f.model.EndpointDial = dial
	}
	return nil
}

func (f *fakeStore) DeleteModel(context.Context, string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = true
	return nil
}

type fakeNodes struct {
	info NodeInfo
	err  error
}

func (f fakeNodes) NodeInfo(context.Context, string) (NodeInfo, error) { return f.info, f.err }

type fakeSecrets struct {
	token      string
	err        error
	deletedKey string
	deleteErr  error
}

func (f *fakeSecrets) Resolve(context.Context, string, string) (string, error) { return f.token, f.err }
func (f *fakeSecrets) DeleteAll(_ context.Context, key string) error {
	f.deletedKey = key
	return f.deleteErr
}

type fakeProber struct{ status Status }

func (f fakeProber) Probe(context.Context, string, string, string) Status { return f.status }

// fakeRuntime embeds docker.Runtime so only the methods the controller
// uses need implementing; anything else panics on the nil embed.
type fakeRuntime struct {
	docker.Runtime
	containers map[string]*docker.ContainerState
	volumes    []string
	created    []docker.ContainerSpec
	started    []string
	removed    []string
	createErr  error
	startErr   error
	removeErr  error
	hostPort   int
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{containers: map[string]*docker.ContainerState{}, hostPort: 40000}
}

func (f *fakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	c := f.containers[name]
	if c == nil {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (f *fakeRuntime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	var out []docker.ContainerState
	for n, c := range f.containers {
		if strings.HasPrefix(n, prefix) {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (f *fakeRuntime) EnsureVolume(_ context.Context, name string) error {
	f.volumes = append(f.volumes, name)
	return nil
}

func (f *fakeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, spec)
	f.containers[spec.Name] = &docker.ContainerState{ID: "id-" + spec.Name, Name: spec.Name}
	return "id-" + spec.Name, nil
}

func (f *fakeRuntime) Start(_ context.Context, id string) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, id)
	for _, c := range f.containers {
		if c.ID == id {
			c.Running = true
			for _, p := range []int{11434, 8000, 8080} {
				c.Ports = append(c.Ports, docker.PortBinding{ContainerPort: p, HostPort: f.hostPort})
			}
		}
	}
	return nil
}

func (f *fakeRuntime) Stop(context.Context, string, time.Duration) error { return nil }

func (f *fakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, id)
	for n, c := range f.containers {
		if c.ID == id {
			delete(f.containers, n)
		}
	}
	return nil
}

var goodNode = NodeInfo{GPU: gpu.Info{Present: true, RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0}, {Index: 1}}}, BindIP: "127.0.0.1", DialHost: "127.0.0.1"}

func testModel() *store.Model {
	return &store.Model{Name: "chat", Engine: EngineOllama, ModelRef: "llama3.1:8b", GPUCount: -1}
}

func reason(t *testing.T, res reconcile.Result) reconcile.Condition {
	t.Helper()
	if len(res.Conditions) != 1 {
		t.Fatalf("conditions = %+v, want exactly one", res.Conditions)
	}
	return res.Conditions[0]
}

func TestController_Reconcile_Lifecycle(t *testing.T) {
	st := &fakeStore{model: testModel()}
	rt := newFakeRuntime()
	c := New("chat", st, fakeNodes{info: goodNode}, rt, fakeProber{Status{Phase: PhaseDownloading, Detail: "downloading llama3.1:8b: 10%"}})

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if cond := reason(t, res); cond.Reason != "Starting" {
		t.Errorf("first reason = %q, want Starting", cond.Reason)
	}
	if len(rt.created) != 1 || len(rt.started) != 1 || len(rt.volumes) != 1 {
		t.Fatalf("created/started/volumes = %d/%d/%d, want 1/1/1", len(rt.created), len(rt.started), len(rt.volumes))
	}
	spec := rt.created[0]
	if spec.GPU == nil || spec.GPU.Count != -1 {
		t.Errorf("GPU request = %+v, want all GPUs", spec.GPU)
	}
	if spec.Volumes[0].ContainerPath != "/root/.ollama" || spec.Volumes[0].Name != "platform-model-chat-cache" {
		t.Errorf("volume = %+v", spec.Volumes[0])
	}
	if spec.Ports[0].HostIP != "127.0.0.1" || spec.Ports[0].ContainerPort != 11434 {
		t.Errorf("port = %+v", spec.Ports[0])
	}
	if spec.Env["OLLAMA_HOST"] == "" {
		t.Error("OLLAMA_HOST not set")
	}

	res, _ = c.Reconcile(context.Background())
	cond := reason(t, res)
	if cond.Reason != "Downloading" || cond.Status != reconcile.ConditionFalse || !strings.Contains(cond.Message, "10%") {
		t.Errorf("second condition = %+v, want Downloading with progress", cond)
	}
	if st.endpoint != "127.0.0.1:40000" {
		t.Errorf("endpoint = %q, want 127.0.0.1:40000", st.endpoint)
	}
	if len(rt.created) != 1 {
		t.Errorf("must not create again, created = %d", len(rt.created))
	}

	tests := []struct {
		status Status
		want   string
		ready  reconcile.ConditionStatus
	}{
		{Status{Phase: PhaseLoading, Detail: "loading"}, "Loading", reconcile.ConditionFalse},
		{Status{Phase: PhaseFailed, Detail: "boom"}, "DownloadFailed", reconcile.ConditionFalse},
		{Status{Phase: PhaseUnreachable, Detail: "starting"}, "Starting", reconcile.ConditionFalse},
		{Status{Phase: PhaseReady}, "ModelLoaded", reconcile.ConditionTrue},
	}
	for _, tt := range tests {
		c.prober = fakeProber{tt.status}
		res, _ := c.Reconcile(context.Background())
		if cond := reason(t, res); cond.Reason != tt.want || cond.Status != tt.ready {
			t.Errorf("phase %s: condition = %+v, want %s/%s", tt.status.Phase, cond, tt.want, tt.ready)
		}
	}
}

func TestController_Reconcile_Blocks(t *testing.T) {
	tests := []struct {
		name    string
		model   func() *store.Model
		node    NodeInfo
		secrets *fakeSecrets
		want    string
	}{
		{name: "no gpu", model: testModel, node: NodeInfo{}, want: "NoGPUOnNode"},
		{name: "runtime missing", model: testModel, node: NodeInfo{GPU: gpu.Info{Present: true}}, want: "GPURuntimeMissing"},
		{name: "too many gpus", model: func() *store.Model { m := testModel(); m.GPUCount = 4; return m }, node: goodNode, want: "InsufficientGPUs"},
		{name: "hf token but no secrets", model: func() *store.Model { m := testModel(); m.HFTokenSet = true; return m }, node: goodNode, want: "HFTokenUnavailable"},
		{name: "hf token unreadable", model: func() *store.Model { m := testModel(); m.HFTokenSet = true; return m }, node: goodNode, secrets: &fakeSecrets{err: errors.New("locked")}, want: "HFTokenUnavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime()
			var opts []Option
			if tt.secrets != nil {
				opts = append(opts, WithSecrets(tt.secrets))
			}
			c := New("chat", &fakeStore{model: tt.model()}, fakeNodes{info: tt.node}, rt, fakeProber{}, opts...)
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if cond := reason(t, res); cond.Reason != tt.want {
				t.Errorf("reason = %q, want %q", cond.Reason, tt.want)
			}
			if len(rt.created) != 0 {
				t.Error("must not create a container while blocked")
			}
		})
	}
}

func TestController_Reconcile_HFTokenReachesEngineEnvOnly(t *testing.T) {
	m := testModel()
	m.Engine, m.ModelRef, m.HFTokenSet = EngineVLLM, "org/model", true
	rt := newFakeRuntime()
	c := New("chat", &fakeStore{model: m}, fakeNodes{info: goodNode}, rt, fakeProber{}, WithSecrets(&fakeSecrets{token: "hf_secret"}))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	env := rt.created[0].Env
	if env["HF_TOKEN"] != "hf_secret" || env["HUGGING_FACE_HUB_TOKEN"] != "hf_secret" {
		t.Errorf("env = %v, want HF token injected", env)
	}
	cmd := strings.Join(rt.created[0].Command, " ")
	if strings.Contains(cmd, "hf_secret") {
		t.Errorf("token leaked into command line: %s", cmd)
	}
	if rt.created[0].ShmSizeBytes == 0 {
		t.Error("vllm needs a larger /dev/shm")
	}
}

// Create succeeded but Start failed: the next pass must start the
// existing container instead of creating a second one.
func TestController_Reconcile_HalfSucceededCreateThenStart(t *testing.T) {
	st := &fakeStore{model: testModel()}
	rt := newFakeRuntime()
	c := New("chat", st, fakeNodes{info: goodNode}, rt, fakeProber{Status{Phase: PhaseLoading}})

	rt.startErr = errors.New("nvidia runtime hiccup")
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("first Reconcile: want start error")
	}
	if cond := reason(t, res); cond.Reason != "StartFailed" {
		t.Errorf("reason = %q, want StartFailed", cond.Reason)
	}
	if len(rt.created) != 1 {
		t.Fatalf("created = %d, want 1", len(rt.created))
	}

	rt.startErr = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if len(rt.created) != 1 {
		t.Errorf("created = %d after recovery, want still 1", len(rt.created))
	}
	if len(rt.started) != 1 {
		t.Errorf("started = %d, want the existing container started once", len(rt.started))
	}
}

func TestController_Reconcile_ConfigChangeRemovesStaleContainer(t *testing.T) {
	st := &fakeStore{model: testModel()}
	rt := newFakeRuntime()
	c := New("chat", st, fakeNodes{info: goodNode}, rt, fakeProber{Status{Phase: PhaseLoading}})
	for i := 0; i < 2; i++ {
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	}
	first := rt.created[0].Name

	st.model.RestartNonce++
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile after restart: %v", err)
	}
	if len(rt.created) != 2 || rt.created[1].Name == first {
		t.Fatalf("expected a new container name, created = %v", rt.created)
	}
	if _, stillThere := rt.containers[first]; stillThere {
		t.Error("old revision container was not removed")
	}
}

func TestController_Reconcile_RemoteNodeWithoutMesh(t *testing.T) {
	node := goodNode
	node.DialHost = ""
	st := &fakeStore{model: testModel()}
	c := New("chat", st, fakeNodes{info: node}, newFakeRuntime(), fakeProber{})
	for i := 0; i < 2; i++ {
		_, _ = c.Reconcile(context.Background())
	}
	res, _ := c.Reconcile(context.Background())
	if cond := reason(t, res); cond.Reason != "EndpointUnreachable" {
		t.Errorf("reason = %q, want EndpointUnreachable", cond.Reason)
	}
	if st.endpoint != "" {
		t.Errorf("endpoint = %q, want none recorded", st.endpoint)
	}
}

func TestController_Reconcile_NoModelAndStoreError(t *testing.T) {
	c := New("chat", &fakeStore{}, fakeNodes{}, newFakeRuntime(), fakeProber{})
	res, err := c.Reconcile(context.Background())
	if err != nil || reason(t, res).Reason != "NoDesiredState" {
		t.Errorf("missing model: res=%+v err=%v", res, err)
	}
	c = New("chat", &fakeStore{err: errors.New("db down")}, fakeNodes{}, newFakeRuntime(), fakeProber{})
	res, err = c.Reconcile(context.Background())
	if err == nil || reason(t, res).Reason != "StoreError" {
		t.Errorf("store error: res=%+v err=%v", res, err)
	}
}

func TestController_Teardown(t *testing.T) {
	t.Run("removes containers, secrets, row", func(t *testing.T) {
		m := testModel()
		m.Deleting = true
		st := &fakeStore{model: m}
		rt := newFakeRuntime()
		rt.containers[ContainerPrefix("", "chat")+"aaaa"] = &docker.ContainerState{ID: "c1", Name: ContainerPrefix("", "chat") + "aaaa", Running: true}
		rt.containers[ContainerPrefix("", "chat-2")+"bbbb"] = &docker.ContainerState{ID: "c2", Name: ContainerPrefix("", "chat-2") + "bbbb"}
		sec := &fakeSecrets{}
		c := New("chat", st, fakeNodes{}, rt, fakeProber{}, WithSecrets(sec))
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if reason(t, res).Reason != "Deleted" || !st.deleted {
			t.Errorf("res=%+v deleted=%v", res, st.deleted)
		}
		if !reflect.DeepEqual(rt.removed, []string{"c1"}) {
			t.Errorf("removed = %v, want only this model's container", rt.removed)
		}
		if sec.deletedKey != "model/chat" {
			t.Errorf("secrets namespace = %q", sec.deletedKey)
		}
	})

	t.Run("half succeeded keeps the row for retry", func(t *testing.T) {
		m := testModel()
		m.Deleting = true
		st := &fakeStore{model: m}
		rt := newFakeRuntime()
		name := ContainerPrefix("", "chat") + "aaaa"
		rt.containers[name] = &docker.ContainerState{ID: "c1", Name: name}
		rt.removeErr = errors.New("docker busy")
		c := New("chat", st, fakeNodes{}, rt, fakeProber{})
		res, err := c.Reconcile(context.Background())
		if err == nil || reason(t, res).Reason != "DeleteFailed" || st.deleted {
			t.Fatalf("res=%+v err=%v deleted=%v, want DeleteFailed and row kept", res, err, st.deleted)
		}
		rt.removeErr = nil
		if _, err := c.Reconcile(context.Background()); err != nil || !st.deleted {
			t.Errorf("retry: err=%v deleted=%v", err, st.deleted)
		}
	})

	t.Run("secrets wipe failure keeps the row", func(t *testing.T) {
		m := testModel()
		m.Deleting = true
		st := &fakeStore{model: m}
		c := New("chat", st, fakeNodes{}, newFakeRuntime(), fakeProber{}, WithSecrets(&fakeSecrets{deleteErr: errors.New("nope")}))
		if _, err := c.Reconcile(context.Background()); err == nil || st.deleted {
			t.Errorf("err=%v deleted=%v, want error and row kept", err, st.deleted)
		}
	})
}
