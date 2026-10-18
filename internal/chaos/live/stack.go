// Package live runs the chaos scenarios against real local Docker containers
// that carry a unique name prefix and are always removed afterwards.
package live

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploywatch"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// BaseImage is the only image the scenarios use, so none of them pulls anything.
const BaseImage = "node:22-alpine"

// appScript answers ok-VERSION and, in MODE=oom, never listens and allocates
// until the kernel kills it.
const appScript = `
const http = require('http');
const v = process.env.VERSION, mode = process.env.MODE;
if (mode === 'oom') {
  const hold = [];
  setInterval(() => hold.push(Buffer.alloc(8 * 1024 * 1024, 1)), 50);
} else {
  http.createServer((req, res) => {
    if (req.url === '/healthz') { res.writeHead(200); res.end('ok'); return; }
    res.writeHead(200); res.end('ok-' + v);
  }).listen(8080);
}
process.on('SIGTERM', () => process.exit(0));
`

// Stack is a control plane slice on a real Docker engine: the application and
// ingress controllers, a rollout gate and a throwaway store.
type Stack struct {
	Prefix  string
	Service string
	Domain  string
	Addr    string

	cli    *dockerclient.Client
	rt     *docker.Client
	db     *store.DB
	proxy  *socketProxy
	logger *slog.Logger
	tmp    string
	engine *reconcile.Engine

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
	driver *ingress.Driver
}

// Available reports why the scenarios cannot run here, or nil when Docker and the base image are present.
func Available(ctx context.Context) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()
	if _, err := cli.Ping(ctx); err != nil {
		return fmt.Errorf("docker daemon not reachable: %w", err)
	}
	if _, err := cli.ImageInspect(ctx, BaseImage); err != nil {
		return fmt.Errorf("%s not present locally: %w", BaseImage, err)
	}
	return nil
}

// NewStack builds and starts a stack. withProxy routes the control plane's Docker traffic through a switchable socket proxy.
func NewStack(ctx context.Context, withProxy bool) (*Stack, error) {
	var b [3]byte
	_, _ = rand.Read(b[:])
	prefix := "lrchaos-" + hex.EncodeToString(b[:])
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	tmp, err := os.MkdirTemp("", prefix)
	if err != nil {
		_ = cli.Close()
		return nil, err
	}
	s := &Stack{
		Prefix: prefix, Service: prefix + "-app", Domain: prefix + ".chaos.internal",
		cli: cli, tmp: tmp, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	var opts []docker.ClientOption
	if withProxy {
		if s.proxy, err = newSocketProxy(tmp, cli.DaemonHost()); err != nil {
			s.Close()
			return nil, fmt.Errorf("socket proxy: %w", err)
		}
		opts = append(opts, docker.WithHost(s.proxy.host()))
	}
	if s.rt, err = docker.NewClient(opts...); err != nil {
		s.Close()
		return nil, fmt.Errorf("docker runtime: %w", err)
	}
	if s.db, err = store.Open(ctx, filepath.Join(tmp, "chaos.db")); err != nil {
		s.Close()
		return nil, fmt.Errorf("open store: %w", err)
	}
	for _, v := range []string{"v1", "v2"} {
		if err := cli.ImageTag(ctx, BaseImage, s.repo()+":"+v); err != nil {
			s.Close()
			return nil, fmt.Errorf("tag %s: %w", v, err)
		}
	}
	if err := s.start(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Stack) repo() string { return s.Prefix + "/app" }

func (s *Stack) start(ctx context.Context) error {
	cfg := deploysafety.Effective{
		SuccessThreshold: 3, StableWindow: 3 * time.Second, DrainTimeout: 3 * time.Second,
		WatchWindow: 60 * time.Second, WatchMaxRestarts: 2, WatchErrorRate: 0.2, WatchMinRequests: 20,
		AutoRollback: true, RollbackPins: 3, WatchInterval: time.Second,
	}
	pinner := &deploywatch.Pinner{Store: s.db, Images: s.rt, Keep: cfg.RollbackPins, Namespace: s.Prefix, Logger: s.logger}
	app := application.New(s.Service, s.db, s.rt,
		application.WithStabilityGate(cfg, s.db), application.WithGateRecorder(s.db),
		application.WithDrainTracker(application.NewDrainTracker()), application.WithRollbackPinner(pinner),
		application.WithGateTick(250*time.Millisecond), application.WithReadyBudget(30*time.Second),
		application.WithRolloutRecorder(s.db),
	)
	if s.Addr == "" {
		port, err := freePort()
		if err != nil {
			return err
		}
		s.Addr = fmt.Sprintf("127.0.0.1:%d", port)
	}
	adminPort, err := freePort()
	if err != nil {
		return err
	}
	s.driver = ingress.New(nil)
	ing := ingressreconcile.New(s.db, s.rt, s.driver,
		ingressreconcile.WithServerName(s.Prefix),
		ingressreconcile.WithListenAddr(s.Addr),
		ingressreconcile.WithAdminListen(fmt.Sprintf("127.0.0.1:%d", adminPort)),
		ingressreconcile.WithStorageDir(filepath.Join(s.tmp, "ingress")),
		ingressreconcile.WithRequestStats(),
		ingressreconcile.WithHardening(ingress.DefaultHardening()),
	)
	engine := reconcile.NewEngine(s.logger, app, ing)
	engine.SetStore(s.db)
	events, _ := s.rt.Events(ctx)
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	s.engine = engine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		_ = engine.Run(runCtx, events, time.Second)
	}()
	return nil
}

// RestartControlPlane stops the reconcile engine and the embedded ingress and starts fresh ones on the same state.
func (s *Stack) RestartControlPlane(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	cancel()
	s.wg.Wait()
	if err := s.driver.Stop(ctx); err != nil {
		return fmt.Errorf("stop ingress: %w", err)
	}
	return s.start(ctx)
}

// Ship records version as the desired release; mode is passed to the fixture app.
func (s *Stack) Ship(ctx context.Context, version, mode string, memoryBytes int64) error {
	svc := store.DesiredService{
		Name: s.Service, Port: 8080, Domains: []string{s.Domain},
		Command: []string{"node", "-e", appScript},
		Env:     map[string]string{"VERSION": version, "MODE": mode},
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{
			Path: "/healthz", Interval: 300 * time.Millisecond, Timeout: time.Second,
		}},
	}
	if memoryBytes > 0 {
		svc.Resources = &store.ServiceResources{MemoryBytes: memoryBytes}
	}
	trigger := deploy.ImageTrigger{Store: s.db, Nudger: s.engine, Source: store.DeployAttemptSourceImage}
	_, err := trigger.Deploy(ctx, svc, s.repo()+":"+version)
	return err
}

// Client dials every request to the local ingress listener.
func (s *Stack) Client() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, s.Addr)
			},
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the local issuer's certificate is never trusted by this process
			MaxIdleConnsPerHost: 64,
		},
	}
}

// URL is the app URL through the ingress.
func (s *Stack) URL() string { return "https://" + s.Domain + "/" }

// WaitServing polls the ingress until the app answers ok-version.
func (s *Stack) WaitServing(ctx context.Context, version string, timeout time.Duration) error {
	c := s.Client()
	return poll(ctx, timeout, func() bool {
		body, status, err := get(ctx, c, s.URL())
		return err == nil && status == http.StatusOK && body == "ok-"+version
	})
}

// Serving returns the version the ingress currently answers with, empty when it does not answer.
func (s *Stack) Serving(ctx context.Context) string {
	body, status, err := get(ctx, s.Client(), s.URL())
	if err != nil || status != http.StatusOK {
		return ""
	}
	return strings.TrimPrefix(body, "ok-")
}

// Running returns the running containers of the app.
func (s *Stack) Running(ctx context.Context) []docker.ContainerState {
	found, err := s.rt.ListByPrefix(ctx, s.Service+"-")
	if err != nil {
		return nil
	}
	var out []docker.ContainerState
	for _, c := range found {
		if c.Running {
			out = append(out, c)
		}
	}
	return out
}

// Attempt returns the deploy attempt for an image version, zero when none exists yet.
func (s *Stack) Attempt(ctx context.Context, version string) store.DeployAttempt {
	attempts, _ := s.db.ListDeployAttempts(ctx, s.Service)
	for _, a := range attempts {
		if a.Image == s.repo()+":"+version {
			return a
		}
	}
	return store.DeployAttempt{}
}

func (s *Stack) removeContainers(ctx context.Context) {
	found, err := s.rt.ListByPrefix(ctx, s.Prefix)
	if err != nil {
		return
	}
	for _, c := range found {
		_ = s.rt.Stop(ctx, c.ID, 2*time.Second)
		_ = s.rt.Remove(ctx, c.ID, true)
	}
}

// Close removes every container and image this stack created, all under its unique prefix.
func (s *Stack) Close() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		s.wg.Wait()
	}
	if s.driver != nil {
		_ = s.driver.Stop(context.Background())
	}
	cctx, stop := context.WithTimeout(context.Background(), 60*time.Second)
	defer stop()
	if s.proxy != nil {
		s.proxy.setDown(false)
	}
	if s.rt != nil {
		s.removeContainers(cctx)
	}
	if s.cli != nil {
		images, _ := s.cli.ImageList(cctx, image.ListOptions{})
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if strings.HasPrefix(tag, s.Prefix) {
					_, _ = s.cli.ImageRemove(cctx, tag, image.RemoveOptions{})
				}
			}
		}
	}
	if s.rt != nil {
		_ = s.rt.Close()
	}
	if s.db != nil {
		_ = s.db.Close()
	}
	if s.proxy != nil {
		s.proxy.close()
	}
	if s.cli != nil {
		_ = s.cli.Close()
	}
	_ = os.RemoveAll(s.tmp)
}

func freePort() (int, error) {
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %v", l.Addr())
	}
	return addr.Port, nil
}

func get(ctx context.Context, c *http.Client, url string) (body string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return string(b), resp.StatusCode, err
}

func poll(ctx context.Context, timeout time.Duration, cond func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out after %s", timeout)
}
