package application

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// memSecrets is the per-service secret store a compose deploy persists
// generated values into and the controller reads at create time.
type memSecrets struct {
	mu   sync.Mutex
	vals map[string]string
}

func (m *memSecrets) Exists(_ context.Context, svc, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.vals[svc+"/"+key]
	return ok, nil
}

func (m *memSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vals[svc+"/"+key]
	if !ok {
		return "", secrets.ErrValueNotFound
	}
	return v, nil
}

type liveStack struct {
	app    string
	db     *store.DB
	rt     *docker.Client
	ctrls  []*Controller
	vols   []string
	prefix string
}

func liveDocker(t *testing.T) *docker.Client {
	t.Helper()
	dockertest.SkipIfShort(t)
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	raw, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := raw.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(); _ = raw.Close() })
	return rt
}

// deployLiveStack runs a compose body through the same steps the API
// deploy path takes (parse, magic vars, translate, save) and builds one
// controller per service.
func deployLiveStack(t *testing.T, rt *docker.Client, app, body string) *liveStack {
	t.Helper()
	ctx := context.Background()
	db := openLiveStore(t)
	sec := &memSecrets{vals: map[string]string{}}
	f, err := compose.Parse([]byte(body))
	if err != nil {
		t.Fatalf("compose.Parse() error = %v", err)
	}
	generated := map[string]string{}
	gen := func(kind, key string, length int) (string, error) {
		if v, ok := generated[kind+key]; ok {
			return v, nil
		}
		v, err := compose.GenerateValue(kind, length)
		generated[kind+key] = v
		return v, err
	}
	persist := func(svcKey, envKey, value string) error {
		sec.vals[app+"-"+svcKey+"/"+envKey] = value
		return nil
	}
	secretEnv, unresolved, err := compose.ResolveMagicVars(f, gen, persist)
	if err != nil || len(unresolved) > 0 {
		t.Fatalf("ResolveMagicVars() = %v, unresolved %v", err, unresolved)
	}
	services, _, err := compose.ToDesiredServices(app, f)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}
	if err := db.SaveApp(ctx, store.App{ID: app, Name: app, CreatedAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}
	s := &liveStack{app: app, db: db, rt: rt, prefix: "lrtest-"}
	for _, svc := range services {
		key := strings.TrimPrefix(svc.Name, app+"-")
		svc.SecretEnv = store.SecretEnvRefsFromNames(secretEnv[key])
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatalf("SaveDesiredService(%s) error = %v", svc.Name, err)
		}
		for _, v := range svc.Volumes {
			s.vols = append(s.vols, v.Name)
		}
		s.ctrls = append(s.ctrls, New(svc.Name, db, rt, WithConditionReader(db), WithSecretResolver(sec),
			WithNetworkPrefix(s.prefix), WithReadyBudget(90*time.Second)))
	}
	cleanupContainers(ctx, t, rt, app+"-")
	t.Cleanup(func() { s.teardown(t) })
	return s
}

func (s *liveStack) pass(ctx context.Context) map[string]reconcile.Condition {
	out := map[string]reconcile.Condition{}
	for _, c := range s.ctrls {
		res, _ := c.Reconcile(ctx)
		_ = s.db.UpsertConditions(ctx, c.Name(), res.Conditions)
		for _, cond := range res.Conditions {
			if cond.Type == reconcile.ConditionTypeReady {
				out[c.serviceName] = cond
			}
		}
	}
	return out
}

// converge runs level-triggered passes until every service is Ready.
func (s *liveStack) converge(t *testing.T, budget time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(budget)
	var last map[string]reconcile.Condition
	for time.Now().Before(deadline) {
		last = s.pass(ctx)
		ready := 0
		for _, c := range last {
			if c.Status == reconcile.ConditionTrue {
				ready++
			}
		}
		if ready == len(s.ctrls) {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("stack %s did not converge within %s: %+v", s.app, budget, last)
}

func (s *liveStack) container(t *testing.T, key string) *docker.ContainerState {
	t.Helper()
	states, err := s.rt.ListByPrefix(context.Background(), s.app+"-"+key+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	for i := range states {
		if states[i].Running && ownsContainer(s.app+"-"+key, states[i].Name) {
			return &states[i]
		}
	}
	return nil
}

func (s *liveStack) teardown(t *testing.T) {
	ctx := context.Background()
	for _, c := range s.ctrls {
		if err := c.Teardown(ctx); err != nil {
			t.Errorf("Teardown(%s) error = %v", c.serviceName, err)
		}
	}
	_ = s.rt.RemoveNetwork(ctx, NetworkName(s.prefix, s.app))
	for _, v := range s.vols {
		_ = s.rt.RemoveVolume(ctx, v)
	}
	if left, _ := s.rt.ListByPrefix(ctx, s.app+"-"); len(left) > 0 {
		t.Errorf("teardown left %d containers behind", len(left))
	}
}

func execOutput(t *testing.T, rt *docker.Client, id string, cmd ...string) string {
	t.Helper()
	rc, err := rt.Exec(context.Background(), id, cmd)
	if err != nil {
		t.Fatalf("Exec(%v) error = %v", cmd, err)
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	return string(b)
}

const liveStackCompose = `
services:
  cache:
    image: redis:7-alpine
    volumes: ["data:/data"]
  api:
    image: nginx:alpine
    ports: ["80"]
    depends_on:
      cache: {condition: service_healthy}
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:80/"]
      interval: 2s
  web:
    image: httpd:alpine
    ports: ["80"]
    depends_on: [api]
`

// TestComposeStack_Live_KillRecoverTeardown is the exit criterion against
// real Docker: a 3-service stack with a volume and a dependency converges,
// a hand-killed container comes back with its data, siblings resolve each
// other by service name, and teardown leaves nothing behind.
func TestComposeStack_Live_KillRecoverTeardown(t *testing.T) {
	rt := liveDocker(t)
	s := deployLiveStack(t, rt, "lrlive-stack", liveStackCompose)
	s.converge(t, 3*time.Minute)

	web := s.container(t, "web")
	if out := execOutput(t, rt, web.ID, "wget", "-q", "-O", "-", "http://api/"); !strings.Contains(out, "nginx") {
		t.Fatalf("web could not reach api by service name, got %q", out)
	}
	cache := s.container(t, "cache")
	execOutput(t, rt, cache.ID, "redis-cli", "set", "marker", "kept")
	execOutput(t, rt, cache.ID, "redis-cli", "save")

	for _, key := range []string{"cache", "api"} {
		killed := s.container(t, key)
		if err := rt.Remove(context.Background(), killed.ID, true); err != nil {
			t.Fatalf("kill %s: %v", key, err)
		}
		s.converge(t, 2*time.Minute)
		back := s.container(t, key)
		if back == nil || back.ID == killed.ID {
			t.Fatalf("%s did not come back as a new container", key)
		}
	}
	cache = s.container(t, "cache")
	if out := execOutput(t, rt, cache.ID, "redis-cli", "get", "marker"); !strings.Contains(out, "kept") {
		t.Errorf("volume data lost across a container kill, redis get marker = %q", out)
	}
}

// TestComposeTemplates_Live installs every shipped manifest template on
// real Docker and checks its healthcheck path answers.
func TestComposeTemplates_Live(t *testing.T) {
	rt := liveDocker(t)
	for _, tpl := range catalog.Templates {
		m := tpl.Manifest
		if m == nil {
			continue
		}
		t.Run(m.ID, func(t *testing.T) {
			in := catalog.InstallInput{Env: map[string]string{}}
			for _, p := range m.Env {
				if p.Required && p.Default == "" && p.Generate == "" {
					in.Env[p.Key] = "live-test"
				}
			}
			body, err := m.Render(in, false)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			s := deployLiveStack(t, rt, "lrtpl-"+m.ID, body)
			s.converge(t, 6*time.Minute)
			hc := s.container(t, m.Healthcheck.Service)
			addr, err := PrimaryAddr(hc, 0)
			if err != nil {
				t.Fatalf("PrimaryAddr() error = %v", err)
			}
			resp, err := http.Get(fmt.Sprintf("http://%s%s", addr, m.Healthcheck.Path)) //nolint:gosec,noctx // test-only loopback probe
			if err != nil {
				t.Fatalf("GET healthcheck: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 500 {
				t.Fatalf("healthcheck %s answered %d", m.Healthcheck.Path, resp.StatusCode)
			}
		})
	}
}
