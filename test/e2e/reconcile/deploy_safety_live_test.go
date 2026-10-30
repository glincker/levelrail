package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploywatch"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	safetyService = "levelrail-test-e2e-dsafe"
	safetyRepo    = "levelrail-test/e2e-dsafe"
	safetyDomain  = "dsafe.levelrail.internal"
	safetyBase    = "node:22-alpine"
)

// safetyStack is a control plane slice: real Docker, the application and
// ingress controllers on a real engine, request metrics and the watch.
type safetyStack struct {
	db     *store.DB
	rt     *docker.Client
	engine *reconcile.Engine
	notes  *[]string
	addr   string
}

// TestDeploySafety_Live_ZeroFailedRequests keeps a client hammering the
// ingress through a good deploy, a deploy that crashes inside its stable
// window, and a deploy the post-deploy watch rolls back for 5xx answers.
func TestDeploySafety_Live_ZeroFailedRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	testenv.RequireFullLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	s := startSafetyStack(ctx, t)
	ship := func(version, mode string) {
		t.Helper()
		svc := store.DesiredService{
			Name: safetyService, Port: 8080, Domains: []string{safetyDomain},
			Command: []string{"node", "-e", safetyAppScript},
			Env:     map[string]string{"VERSION": version, "MODE": mode},
			Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{
				Path: "/healthz", Interval: 300 * time.Millisecond, Timeout: time.Second,
			}},
		}
		trigger := deploy.ImageTrigger{Store: s.db, Nudger: s.engine, Source: store.DeployAttemptSourceImage}
		if _, err := trigger.Deploy(ctx, svc, safetyRepo+":"+version); err != nil {
			t.Fatalf("deploy %s: %v", version, err)
		}
	}

	ship("v1", "ok")
	client := ingressClient(s.addr)
	url := "https://" + safetyDomain + "/"
	waitFor(t, 90*time.Second, "v1 to serve through the ingress", func() bool {
		body, status, err := doGet(client, url)
		return err == nil && status == 200 && body == "ok-v1"
	})

	load := newLoadResult()
	loadCtx, stopLoad := context.WithCancel(ctx)
	wg := hammer(loadCtx, client, url, 4, load)
	defer func() { stopLoad(); wg.Wait() }()
	time.Sleep(time.Second)

	t.Run("good deploy", func(t *testing.T) {
		ship("v2", "ok")
		waitFor(t, 90*time.Second, "v2 to take traffic", func() bool { return load.lastVersion() == "v2" })
		waitFor(t, 30*time.Second, "the drain to finish", func() bool { return s.hasPhase(ctx, "v2", deploysafety.PhaseDrain) })
		time.Sleep(2 * time.Second)
		s.assertNoFailures(t, load)
		s.assertPhases(ctx, t, "v2", deploysafety.PhaseReadiness, deploysafety.PhaseStability, deploysafety.PhaseCutover, deploysafety.PhasePin)
	})

	t.Run("crash inside the stable window", func(t *testing.T) {
		ship("v3", "crash")
		waitFor(t, 90*time.Second, "v3 to fail its gate", func() bool {
			a := s.attempt(ctx, t, "v3")
			return a.Status == store.DeployAttemptStatusFailed && slices.Contains(deploysafety.GateFailureReasons, a.Reason)
		})
		time.Sleep(3 * time.Second)
		if v := load.lastVersion(); v != "v2" {
			t.Errorf("serving %q after the failed deploy, want v2 to keep serving", v)
		}
		s.assertNoFailures(t, load)
	})

	t.Run("auto rollback on a 5xx spike", func(t *testing.T) {
		ship("v4", "fail500")
		waitFor(t, 90*time.Second, "v4 to take traffic", func() bool { return load.lastVersion() == "bad-v4" })
		waitFor(t, 120*time.Second, "the watch to roll v4 back", func() bool {
			a := s.attempt(ctx, t, "v4")
			return a.Status == store.DeployAttemptStatusFailed && a.Reason == deploysafety.ReasonWatchErrorRate && s.hasPhase(ctx, "v4", deploysafety.PhaseRollback)
		})
		_, before, _ := load.snapshot()
		waitFor(t, 90*time.Second, "v2 to serve again after the rollback", func() bool { return load.lastVersion() == "v2" })
		time.Sleep(3 * time.Second)
		_, after, _ := load.snapshot()
		if before == 0 {
			t.Errorf("no 5xx answers from v4 were seen, the rollback trigger was not exercised")
		}
		t.Logf("5xx answers from the bad release: %d before rollback, %d total", before, after)
		s.assertNoFailures(t, load)
		svc, err := s.db.GetDesiredService(ctx, safetyService)
		if err != nil || !strings.HasPrefix(svc.Image, "lrtest-rollback/") || svc.Env["VERSION"] != "v2" {
			t.Errorf("desired after rollback = %+v (err %v), want the pinned v2 image with v2's config", svc, err)
		}
		if len(*s.notes) == 0 || !strings.Contains(strings.Join(*s.notes, "\n"), "rolled back automatically") {
			t.Errorf("notifications = %v, want an automatic rollback alert", *s.notes)
		}
	})

	stopLoad()
	wg.Wait()
	ok, appErrors, failures := load.snapshot()
	t.Logf("requests %d, ok by version %v, app 5xx %d, failed %d", load.total.Load(), ok, appErrors, len(failures))
}

// TestDeploySafety_Live_RestartAndOperatorRollback keeps a client on the
// ingress through a deploy, a restart and an explicit rollback, and checks
// the rollback takes traffic promptly and older releases do not linger.
func TestDeploySafety_Live_RestartAndOperatorRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	testenv.RequireFullLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	s := startSafetyStack(ctx, t)
	svcFor := func(version string) store.DesiredService {
		return store.DesiredService{
			Name: safetyService, Port: 8080, Domains: []string{safetyDomain},
			Command: []string{"node", "-e", safetyAppScript},
			Env:     map[string]string{"VERSION": version, "MODE": "ok"},
			Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{
				Path: "/healthz", Interval: 300 * time.Millisecond, Timeout: time.Second,
			}},
		}
	}
	ship := func(version string) {
		t.Helper()
		trigger := deploy.ImageTrigger{Store: s.db, Nudger: s.engine, Source: store.DeployAttemptSourceImage}
		if _, err := trigger.Deploy(ctx, svcFor(version), safetyRepo+":"+version); err != nil {
			t.Fatalf("deploy %s: %v", version, err)
		}
	}

	ship("v1")
	client := ingressClient(s.addr)
	url := "https://" + safetyDomain + "/"
	waitFor(t, 90*time.Second, "v1 to serve through the ingress", func() bool {
		body, status, err := doGet(client, url)
		return err == nil && status == 200 && body == "ok-v1"
	})
	load := newLoadResult()
	loadCtx, stopLoad := context.WithCancel(ctx)
	wg := hammer(loadCtx, client, url, 4, load)
	defer func() { stopLoad(); wg.Wait() }()

	ship("v2")
	waitFor(t, 90*time.Second, "v2 to take traffic", func() bool { return load.lastVersion() == "v2" })
	waitFor(t, 30*time.Second, "the v2 drain to finish", func() bool { return s.hasPhase(ctx, "v2", deploysafety.PhaseDrain) })
	s.assertNoFailures(t, load)

	t.Run("restart", func(t *testing.T) {
		if err := s.db.RestartService(ctx, safetyService); err != nil {
			t.Fatalf("RestartService() error = %v", err)
		}
		s.engine.Nudge()
		svc, err := s.db.GetDesiredService(ctx, safetyService)
		if err != nil {
			t.Fatal(err)
		}
		target := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)
		waitFor(t, 90*time.Second, "the restarted container to run", func() bool {
			st, err := s.rt.InspectByName(ctx, target)
			return err == nil && st != nil && st.Running
		})
		time.Sleep(5 * time.Second)
		s.assertNoFailures(t, load)
	})

	t.Run("operator rollback", func(t *testing.T) {
		v1 := s.attempt(ctx, t, "v1")
		pins, err := s.db.ListRollbackPins(ctx, safetyService)
		if err != nil {
			t.Fatal(err)
		}
		var pin *store.RollbackPin
		for i := range pins {
			if pins[i].DeployAttemptID == v1.ID {
				pin = &pins[i]
			}
		}
		current, err := s.db.GetDesiredService(ctx, safetyService)
		if err != nil {
			t.Fatal(err)
		}
		restored, ref, _ := deploy.RollbackDesired(*current, v1, pin)
		trigger := deploy.ImageTrigger{Store: s.db, Nudger: s.engine, Source: store.DeployAttemptSourceImage, Reason: "RollbackTo: " + v1.ID}
		asked := time.Now()
		if _, err := trigger.Deploy(ctx, restored, ref); err != nil {
			t.Fatalf("rollback deploy: %v", err)
		}
		waitFor(t, 60*time.Second, "the rollback to take traffic", func() bool { return load.lastVersion() == "v1" })
		t.Logf("rollback took traffic %s after it was asked", time.Since(asked).Round(100*time.Millisecond))
		time.Sleep(5 * time.Second)
		s.assertNoFailures(t, load)

		// The drain and the watch window bound how long the newer release stays.
		limit := safetyConfig().DrainTimeout + safetyConfig().WatchWindow + 30*time.Second
		waitFor(t, limit, "older releases to be retired", func() bool {
			all, err := s.rt.ListByPrefix(ctx, safetyService+"-")
			if err != nil {
				return false
			}
			running := 0
			for _, cs := range all {
				if cs.Running && application.OwnsContainer(safetyService, cs.Name) {
					running++
				}
			}
			return running <= 2
		})
		s.assertNoFailures(t, load)
	})

	stopLoad()
	wg.Wait()
	ok, appErrors, failures := load.snapshot()
	t.Logf("requests %d, ok by version %v, app 5xx %d, failed %d", load.total.Load(), ok, appErrors, len(failures))
}

func safetyConfig() deploysafety.Effective {
	return deploysafety.Effective{
		SuccessThreshold: 3, StableWindow: 3 * time.Second, DrainTimeout: 3 * time.Second,
		WatchWindow: 60 * time.Second, WatchMaxRestarts: 2, WatchErrorRate: 0.2, WatchMinRequests: 20,
		WatchMode: deploysafety.WatchModeAuto, WatchSoftTicks: 3, RollbackPins: 3, WatchInterval: time.Second,
	}
}

func startSafetyStack(ctx context.Context, t *testing.T) *safetyStack {
	t.Helper()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	if _, err := cli.ImageInspect(ctx, safetyBase); err != nil {
		t.Skipf("%s not present locally (scripts/prepull-test-images.sh): %v", safetyBase, err)
	}
	rt, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	for _, v := range []string{"v1", "v2", "v3", "v4", "v5"} {
		if err := cli.ImageTag(ctx, safetyBase, safetyRepo+":"+v); err != nil {
			t.Fatalf("tag %s: %v", v, err)
		}
	}
	cleanupContainers(ctx, t, rt, safetyService)
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cleanupContainers(cctx, t, rt, safetyService)
		images, _ := cli.ImageList(cctx, image.ListOptions{})
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if strings.HasPrefix(tag, safetyRepo+":") || strings.HasPrefix(tag, "lrtest-rollback/") {
					_, _ = cli.ImageRemove(cctx, tag, image.RemoveOptions{})
				}
			}
		}
	})

	db := openLiveStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := safetyConfig()
	pinner := &deploywatch.Pinner{Store: db, Images: rt, Keep: cfg.RollbackPins, Namespace: "lrtest", Logger: logger}
	app := application.New(safetyService, db, rt,
		application.WithStabilityGate(cfg, db), application.WithGateRecorder(db),
		application.WithDrainTracker(application.NewDrainTracker()), application.WithRollbackPinner(pinner),
		application.WithGateTick(250*time.Millisecond), application.WithReadyBudget(30*time.Second),
		application.WithRolloutRecorder(db),
	)
	s := &safetyStack{db: db, rt: rt, notes: &[]string{}}
	ing := s.newIngress(t)
	s.engine = reconcile.NewEngine(logger, app, ing)
	s.engine.SetStore(db)
	events, _ := rt.Events(ctx)
	go func() { _ = s.engine.Run(ctx, events, time.Second) }()

	tdb, err := telemetry.Open(ctx, filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = tdb.Close() })
	go telemetry.NewRequestSampler(ingress.DefaultRequestStats(), tdb, time.Second, logger).Run(ctx)

	tracker := alerting.NewRestartTracker()
	go func() { _ = tracker.Run(ctx, rt, db, time.Second, logger) }()

	var mu sync.Mutex
	watcher := &deploywatch.Watcher{
		Store: db, Audit: db, Restarts: tracker, Defaults: cfg, Metrics: telemetry.NewLocalFederator(tdb), Nudger: s.engine, Logger: logger,
		SampleInterval: time.Second,
		Inspectors:     func(string) (docker.ExitStateInspector, error) { return rt, nil },
		Notify: func(_ context.Context, _, _, message string) {
			mu.Lock()
			defer mu.Unlock()
			*s.notes = append(*s.notes, message)
		},
	}
	go watcher.Run(ctx, cfg.WatchInterval)
	return s
}

func (s *safetyStack) newIngress(t *testing.T) *ingressreconcile.Controller {
	t.Helper()
	s.addr = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	driver := ingress.New(nil)
	t.Cleanup(func() { _ = driver.Stop(context.Background()) })
	return ingressreconcile.New(s.db, s.rt, driver,
		ingressreconcile.WithServerName("e2e-dsafe"),
		ingressreconcile.WithListenAddr(s.addr),
		ingressreconcile.WithAdminListen(fmt.Sprintf("127.0.0.1:%d", freePort(t))),
		ingressreconcile.WithStorageDir(t.TempDir()),
		ingressreconcile.WithRequestStats(),
		ingressreconcile.WithHardening(ingress.DefaultHardening()),
	)
}

func (s *safetyStack) attempt(ctx context.Context, t *testing.T, version string) store.DeployAttempt {
	t.Helper()
	attempts, err := s.db.ListDeployAttempts(ctx, safetyService)
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	for _, a := range attempts {
		if a.Image == safetyRepo+":"+version {
			return a
		}
	}
	return store.DeployAttempt{}
}

func (s *safetyStack) events(ctx context.Context, version string) []store.DeployGateEvent {
	attempts, _ := s.db.ListDeployAttempts(ctx, safetyService)
	for _, a := range attempts {
		if a.Image == safetyRepo+":"+version {
			events, _ := s.db.ListGateEvents(ctx, a.ID)
			return events
		}
	}
	return nil
}

func (s *safetyStack) hasPhase(ctx context.Context, version, phase string) bool {
	for _, e := range s.events(ctx, version) {
		if e.Phase == phase {
			return true
		}
	}
	return false
}

func (s *safetyStack) assertPhases(ctx context.Context, t *testing.T, version string, phases ...string) {
	t.Helper()
	for _, p := range phases {
		if !s.hasPhase(ctx, version, p) {
			t.Errorf("%s timeline = %+v, missing phase %s", version, s.events(ctx, version), p)
		}
	}
}

func (s *safetyStack) assertNoFailures(t *testing.T, load *loadResult) {
	t.Helper()
	ok, appErrors, failures := load.snapshot()
	if len(failures) > 0 {
		t.Fatalf("%d of %d requests failed (ok %v, app 5xx %d), first: %v", len(failures), load.total.Load(), ok, appErrors, failures[:min(len(failures), 10)])
	}
}
