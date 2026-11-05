// TestRollbackCutover_Live_NoDroppedRequests measures whether an explicit
// operator rollback, issued through the real HTTP API while a client keeps
// requesting the app through a real ingress, drops any request. It only
// observes and asserts; it does not change the reconciler or ingress.
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploywatch"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	rbService  = "levelrail-test-e2e-rbcut"
	rbRepo     = "levelrail-test/e2e-rbcut"
	rbDomain   = "rbcut.levelrail.internal"
	rbBaseImg  = "node:22-alpine"
	rbAdmin    = "e2e-rbcut-admin"
	rbAdminPwd = "e2e-rbcut-correct-horse" //nolint:gosec // test fixture credential, not a real secret

	rbEnvRPS          = "E2E_ROLLBACK_RPS"
	rbEnvStableWindow = "E2E_ROLLBACK_STABLE_WINDOW"
	rbEnvTimeout      = "E2E_ROLLBACK_TIMEOUT"
	rbDefaultRPS      = 50
	rbDefaultStable   = 5 * time.Second
	rbDefaultTimeout  = 4 * time.Minute
)

// rbAppScript serves ok-VERSION and a readiness path, nothing else.
const rbAppScript = `
const http = require('http');
const v = process.env.VERSION;
http.createServer((req, res) => {
  if (req.url === '/healthz') { res.writeHead(200); res.end('ok'); return; }
  res.writeHead(200); res.end('ok-' + v);
}).listen(8080);
process.on('SIGTERM', () => process.exit(0));
`

func rbEnvInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

func rbEnvDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(name)); err == nil && d > 0 {
		return d
	}
	return def
}

// rbProbe is one observed request, in completion order.
type rbProbe struct {
	at      time.Time
	version string
	failure string
}

type rbLog struct {
	mu     sync.Mutex
	probes []rbProbe
}

func (l *rbLog) add(p rbProbe) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.probes = append(l.probes, p)
}

func (l *rbLog) snapshot() []rbProbe {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]rbProbe(nil), l.probes...)
}

// rbFire sends one request and classifies it: only a 2xx with an
// "ok-<version>" body counts as served.
func rbFire(ctx context.Context, client *http.Client, url string) rbProbe {
	p := rbProbe{at: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		p.failure = "build request: " + err.Error()
		return p
	}
	resp, err := client.Do(req)
	if err != nil {
		p.failure = "transport: " + err.Error()
		return p
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	switch {
	case err != nil:
		p.failure = "read body: " + err.Error()
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		p.failure = fmt.Sprintf("status %d", resp.StatusCode)
	case len(body) == 0:
		p.failure = "empty body"
	case !strings.HasPrefix(string(body), "ok-"):
		p.failure = fmt.Sprintf("unexpected body %.40q", body)
	default:
		p.version = strings.TrimPrefix(string(body), "ok-")
	}
	return p
}

// rbDrive fires requests at a fixed rate until ctx ends, each in its own
// goroutine so a slow answer never lowers the offered rate.
func rbDrive(ctx context.Context, client *http.Client, url string, rps int, out *rbLog) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second / time.Duration(rps))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				wg.Add(1)
				go func() {
					defer wg.Done()
					p := rbFire(ctx, client, url)
					if ctx.Err() != nil && p.failure != "" {
						return
					}
					out.add(p)
				}()
			}
		}
	}()
	return &wg
}

func rbWait(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

func TestRollbackCutover_Live_NoDroppedRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	testenv.RequireFullLive(t)
	rps := rbEnvInt(rbEnvRPS, rbDefaultRPS)
	stable := rbEnvDuration(rbEnvStableWindow, rbDefaultStable)
	ctx, cancel := context.WithTimeout(context.Background(), rbEnvDuration(rbEnvTimeout, rbDefaultTimeout))
	defer cancel()

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	if _, err := cli.ImageInspect(ctx, rbBaseImg); err != nil {
		t.Skipf("%s not present locally (scripts/prepull-test-images.sh): %v", rbBaseImg, err)
	}
	rt, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	for _, v := range []string{"v1", "v2"} {
		if err := cli.ImageTag(ctx, rbBaseImg, rbRepo+":"+v); err != nil {
			t.Fatalf("tag %s: %v", v, err)
		}
	}
	cleanupContainers(ctx, t, rt, rbService)
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		cleanupContainers(cctx, t, rt, rbService)
		images, _ := cli.ImageList(cctx, image.ListOptions{})
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if strings.HasPrefix(tag, rbRepo+":") || strings.HasPrefix(tag, "lrtest-rbcut/") {
					_, _ = cli.ImageRemove(cctx, tag, image.RemoveOptions{})
				}
			}
		}
	})

	db := openLiveStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := deploysafety.Effective{
		SuccessThreshold: 3, StableWindow: 3 * time.Second, DrainTimeout: 3 * time.Second,
		WatchWindow: time.Minute, WatchMaxRestarts: 2, WatchErrorRate: 0.2, WatchMinRequests: 20,
		WatchMode: deploysafety.WatchModeAuto, WatchSoftTicks: 3, RollbackPins: 3, WatchInterval: time.Second,
	}
	pinner := &deploywatch.Pinner{Store: db, Images: rt, Keep: cfg.RollbackPins, Namespace: "lrtest-rbcut", Logger: logger}
	app := application.New(rbService, db, rt,
		application.WithStabilityGate(cfg, db), application.WithGateRecorder(db),
		application.WithDrainTracker(application.NewDrainTracker()), application.WithRollbackPinner(pinner),
		application.WithGateTick(250*time.Millisecond), application.WithReadyBudget(30*time.Second),
		application.WithRolloutRecorder(db),
	)
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	driver := ingress.New(nil)
	t.Cleanup(func() { _ = driver.Stop(context.Background()) })
	ing := ingressreconcile.New(db, rt, driver,
		ingressreconcile.WithServerName("e2e-rbcut"),
		ingressreconcile.WithListenAddr(addr),
		ingressreconcile.WithAdminListen(fmt.Sprintf("127.0.0.1:%d", freePort(t))),
		ingressreconcile.WithStorageDir(t.TempDir()),
		ingressreconcile.WithHardening(ingress.DefaultHardening()),
	)
	engine := reconcile.NewEngine(logger, app, ing)
	engine.SetStore(db)
	events, _ := rt.Events(ctx)
	go func() { _ = engine.Run(ctx, events, time.Second) }()

	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, db,
		api.WithDeploySafety(db, rt), api.WithDeployGate(db, cfg), api.WithReconcileNudger(engine))
	ts := newE2ETestServer(t, router)
	if err := api.BootstrapAdmin(ctx, db, rbAdmin, rbAdminPwd); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, ts.URL, rbAdmin, rbAdminPwd)

	ship := func(version string) {
		t.Helper()
		svc := store.DesiredService{
			Name: rbService, Port: 8080, Domains: []string{rbDomain},
			Command: []string{"node", "-e", rbAppScript},
			Env:     map[string]string{"VERSION": version},
			Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{
				Path: "/healthz", Interval: 300 * time.Millisecond, Timeout: time.Second,
			}},
		}
		trigger := deploy.ImageTrigger{Store: db, Nudger: engine, Source: store.DeployAttemptSourceImage}
		if _, err := trigger.Deploy(ctx, svc, rbRepo+":"+version); err != nil {
			t.Fatalf("deploy %s: %v", version, err)
		}
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(dctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(dctx, network, addr)
			},
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Caddy's internal issuer is never trusted by this process
			MaxIdleConnsPerHost: rps,
		},
	}
	url := "https://" + rbDomain + "/"

	ship("v1")
	rbWait(t, 90*time.Second, "v1 to serve through the ingress", func() bool {
		p := rbFire(ctx, client, url)
		return p.failure == "" && p.version == "v1"
	})
	ship("v2")
	// The rollback route is keyed by v1's attempt id.
	var v1ID string
	attempts, err := db.ListDeployAttempts(ctx, rbService)
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	for _, a := range attempts {
		if a.Image == rbRepo+":v1" {
			v1ID = a.ID
		}
	}
	if v1ID == "" {
		t.Fatalf("no deploy attempt recorded for v1 in %+v", attempts)
	}
	rbWait(t, 90*time.Second, "v2 to serve through the ingress", func() bool {
		p := rbFire(ctx, client, url)
		return p.failure == "" && p.version == "v2"
	})

	load := &rbLog{}
	loadCtx, stopLoad := context.WithCancel(ctx)
	wg := rbDrive(loadCtx, client, url, rps, load)
	defer func() { stopLoad(); wg.Wait() }()
	time.Sleep(2 * time.Second)

	asked := time.Now()
	status, body := postJSON(t, admin, ts.URL+"/api/v1/apps/"+rbService+"/deploys/"+v1ID+"/rollback", "")
	if status != http.StatusAccepted {
		t.Fatalf("POST rollback: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}

	// Done once v1 has answered and nothing else has been seen for the
	// stable window, so a late flap back to v2 is still caught.
	rbWait(t, 90*time.Second, "only v1 for the stable window", func() bool {
		lastOther := asked
		sawV1 := false
		for _, p := range load.snapshot() {
			if p.at.Before(asked) {
				continue
			}
			if p.failure == "" && p.version == "v1" {
				sawV1 = true
				continue
			}
			lastOther = p.at
		}
		return sawV1 && time.Since(lastOther) >= stable
	})
	stopLoad()
	wg.Wait()

	var order, failures []string
	probes := load.snapshot()
	for _, p := range probes {
		label := p.version
		if p.failure != "" {
			failures = append(failures, p.at.Format("15:04:05.000")+" "+p.failure)
			label = "FAIL"
		}
		if len(order) == 0 || order[len(order)-1] != label {
			order = append(order, label)
		}
	}
	t.Logf("rps %d, requests %d, dropped %d, version order %v", rps, len(probes), len(failures), order)
	if len(failures) > 0 {
		t.Errorf("rollback dropped %d of %d requests, first: %v", len(failures), len(probes), failures[:min(len(failures), 10)])
	}
}
