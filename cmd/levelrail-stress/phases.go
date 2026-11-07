package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/stressagent"
	"github.com/GLINCKER/levelrail/internal/stressprofile"
	"github.com/GLINCKER/levelrail/internal/stressseed"
)

const (
	adminEmail    = "stress@example.com"
	adminPassword = "stress-pass-123456"
	dbFile        = "levelrail.db"
)

// Check is one budget comparison.
type Check struct {
	Name   string  `json:"name"`
	Limit  float64 `json:"limit"`
	Actual float64 `json:"actual"`
	Pass   bool    `json:"pass"`
}

// Report is the JSON result of a run.
type Report struct {
	Profile     string         `json:"profile"`
	Host        string         `json:"host"`
	StartedAt   time.Time      `json:"started_at"`
	DurationSec float64        `json:"duration_seconds"`
	SeedSec     float64        `json:"seed_seconds"`
	Apps        int            `json:"apps"`
	Digest      string         `json:"seed_digest"`
	Nodes       int            `json:"fleet_nodes"`
	Containers  int            `json:"fleet_containers"`
	Sessions    int64          `json:"fleet_sessions"`
	WatchEvents int64          `json:"watch_events"`
	Deploys     int64          `json:"deploys_sent"`
	LogIngest   string         `json:"log_ingest"`
	Resources   Resources      `json:"resources"`
	Endpoints   []EndpointStat `json:"endpoints"`
	CPErrors    []string       `json:"control_plane_errors,omitempty"`
	Checks      []Check        `json:"checks"`
	Pass        bool           `json:"pass"`
}

func (r *Report) summary(w io.Writer) {
	if r == nil {
		return
	}
	p := func(f string, a ...any) { _, _ = fmt.Fprintf(w, f+"\n", a...) }
	p("stress %s on %s: %d apps, %d fake nodes (%d containers), seed %.1fs, run %.0fs", r.Profile, r.Host, r.Apps, r.Nodes, r.Containers, r.SeedSec, r.DurationSec)
	p("%-34s %6s %5s %8s %8s %8s", "endpoint", "n", "err", "p50ms", "p95ms", "p99ms")
	for _, e := range r.Endpoints {
		p("%-34s %6d %5d %8s %8s %8s", e.Endpoint, e.Count, e.Errors, fmtMs(e.P50Ms), fmtMs(e.P95Ms), fmtMs(e.P99Ms))
	}
	p("rss max %.0f MB, goroutines max %d, sqlite max %.1f MB, reconcile pass max %.1f ms (p95 %.1f, %d passes)",
		r.Resources.MaxRSSMB, r.Resources.MaxGoroutines, r.Resources.MaxSQLiteMB, r.Resources.MaxReconcilePassMs, r.Resources.ReconcileP95Ms, r.Resources.ReconcilePasses)
	p("watch events %d, deploys sent %d, control plane ERROR lines %d, log ingest: %s", r.WatchEvents, r.Deploys, len(r.CPErrors), r.LogIngest)
	for _, c := range r.Checks {
		v := "PASS"
		if !c.Pass {
			v = "FAIL"
		}
		p("  %s %-26s actual %.1f limit %.1f", v, c.Name, c.Actual, c.Limit)
	}
	if r.Pass {
		p("RESULT: PASS")
	} else {
		p("RESULT: FAIL")
	}
}

func freePort() (int, error) {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func cpEnv(dataDir, sock string, ports [5]int) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "APP_") || strings.HasPrefix(kv, "DOCKER_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"APP_DATA_DIR="+dataDir,
		fmt.Sprintf("APP_HTTP_ADDR=127.0.0.1:%d", ports[0]),
		fmt.Sprintf("APP_AGENT_ADDR=127.0.0.1:%d", ports[1]),
		fmt.Sprintf("APP_PPROF_ADDR=127.0.0.1:%d", ports[2]),
		fmt.Sprintf("APP_INGRESS_HTTP_ADDR=127.0.0.1:%d", ports[3]),
		fmt.Sprintf("APP_INGRESS_HTTPS_ADDR=127.0.0.1:%d", ports[4]),
		"DOCKER_HOST=unix://"+sock,
	)
}

func execute(ctx context.Context, prof stressprofile.Profile, o options, stderr io.Writer) (*Report, error) {
	tmp, err := os.MkdirTemp("", "levelrail-stress-")
	if err != nil {
		return nil, err
	}
	if !o.keep {
		defer func() { _ = os.RemoveAll(tmp) }()
	}
	bin := o.cpBin
	if bin == "" {
		bin = filepath.Join(tmp, "cp")
		build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", bin, "./cmd/levelrail") //nolint:gosec // fixed argv
		build.Dir = o.repoRoot
		if out, berr := build.CombinedOutput(); berr != nil {
			return nil, fmt.Errorf("build control plane: %w: %s", berr, out)
		}
	}
	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	var ports [5]int
	for i := range ports {
		if ports[i], err = freePort(); err != nil {
			return nil, err
		}
	}
	logPath := filepath.Join(tmp, "cp.log")
	logFile, err := os.Create(logPath) //nolint:gosec // scratch path
	if err != nil {
		return nil, err
	}
	defer func() { _ = logFile.Close() }()
	env := cpEnv(dataDir, filepath.Join(tmp, "no-docker.sock"), ports)
	cp := exec.Command(bin) //nolint:gosec,noctx // the child must outlive request contexts and is stopped explicitly
	cp.Env, cp.Stdout, cp.Stderr = env, logFile, logFile
	ownProcessGroup(cp)
	if err := cp.Start(); err != nil {
		return nil, fmt.Errorf("start control plane: %w", err)
	}
	exited := make(chan struct{})
	go func() { _ = cp.Wait(); close(exited) }()
	defer func() {
		signalGroup(cp, syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
			signalGroup(cp, syscall.SIGKILL)
			<-exited
		}
	}()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	api := fmt.Sprintf("http://127.0.0.1:%d", ports[0])
	agentAddr := fmt.Sprintf("127.0.0.1:%d", ports[1])
	if err := waitReady(ctx, client, api, exited); err != nil {
		return nil, err
	}
	if err := bootstrap(ctx, client, api, bin, env); err != nil {
		return nil, err
	}

	rep := &Report{Profile: prof.Name, Host: runtime.GOOS + "/" + runtime.GOARCH, StartedAt: time.Now().UTC(),
		LogIngest: "not driven: no log ingest endpoint is exercised yet", Apps: prof.Apps}
	token := func(c context.Context, name string) (string, error) {
		body, _ := json.Marshal(map[string]string{"control_plane_addr": agentAddr, "node_name": name})
		req, _ := http.NewRequestWithContext(c, http.MethodPost, api+"/api/v1/nodes/join-tokens", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, derr := client.Do(req)
		if derr != nil {
			return "", derr
		}
		defer func() { _ = resp.Body.Close() }()
		var out struct {
			Token string `json:"token"`
		}
		if resp.StatusCode != http.StatusCreated || json.NewDecoder(resp.Body).Decode(&out) != nil || out.Token == "" {
			return "", fmt.Errorf("join token: status %d", resp.StatusCode)
		}
		return out.Token, nil
	}
	fleet, err := stressagent.Start(ctx, stressagent.Config{Addr: agentAddr, Count: prof.Nodes, NamePrefix: "stress", Token: token,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		return rep, err
	}
	defer fleet.Stop()

	seedStart := time.Now()
	db, err := store.Open(ctx, filepath.Join(dataDir, dbFile))
	if err != nil {
		return rep, fmt.Errorf("open store for seeding: %w", err)
	}
	m, err := stressseed.Seed(ctx, db, stressseed.Config{Apps: prof.Apps, Databases: prof.Databases, Deploys: prof.SeededDeploys,
		EnvsPerApp: prof.EnvsPerApp, DomainsPerApp: prof.DomainsPerApp, Seed: prof.Seed, NodeIDs: fleet.NodeIDs()})
	_ = db.Close()
	if err != nil {
		return rep, err
	}
	rep.SeedSec, rep.Digest = time.Since(seedStart).Seconds(), m.Digest

	rec := newRecorder()
	smp := &sampler{pid: cp.Process.Pid, dataDir: dataDir, pprof: fmt.Sprintf("http://127.0.0.1:%d", ports[2]), api: api, client: client}
	sctx, scancel := context.WithCancel(ctx)
	var swg sync.WaitGroup
	swg.Add(1)
	go func() { defer swg.Done(); smp.run(sctx) }()

	steady, churn, load := prof.PhaseSplit()
	runStart := time.Now()
	var deploys, events atomic.Int64
	apps := m.Apps
	rng := rand.New(rand.NewPCG(prof.Seed, 1)) //nolint:gosec,forbidigo // deterministic load shape, not security

	phase := func(name string, d time.Duration, work func(context.Context)) {
		_, _ = fmt.Fprintf(stderr, "phase %s (%s)\n", name, d.Round(time.Second))
		pctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		work(pctx)
	}
	deployLoop := func(pctx context.Context, wg *sync.WaitGroup) {
		if prof.DeploysPerMinute == 0 {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(time.Minute / time.Duration(prof.DeploysPerMinute))
			defer t.Stop()
			for n := 0; ; n++ {
				select {
				case <-pctx.Done():
					return
				case <-t.C:
					app := apps[rng.IntN(len(apps))] //nolint:forbidigo // deterministic load shape
					body, _ := json.Marshal(map[string]string{"image": fmt.Sprintf("registry.invalid/stress/app:t%d", n)})
					rec.do(pctx, client, "POST /api/v1/apps/{name}/deploys", http.MethodPost, api+"/api/v1/apps/"+app+"/deploys", body)
					deploys.Add(1)
				}
			}
		}()
	}
	list := func(pctx context.Context, wg *sync.WaitGroup, clients int, pause time.Duration) {
		urls := [][2]string{{"GET /api/v2/apps?limit=50", "/api/v2/apps?limit=50"}, {"GET /api/v1/apps", "/api/v1/apps"},
			{"GET /api/v1/databases", "/api/v1/databases"}, {"GET /api/v1/nodes", "/api/v1/nodes"}}
		for c := 0; c < clients; c++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := c; pctx.Err() == nil; i++ {
					u := urls[i%len(urls)]
					rec.do(pctx, client, u[0], http.MethodGet, api+u[1], nil)
					select {
					case <-pctx.Done():
					case <-time.After(pause):
					}
				}
			}()
		}
	}

	phase("steady", steady, func(pctx context.Context) {
		var wg sync.WaitGroup
		list(pctx, &wg, 1, 500*time.Millisecond)
		wg.Wait()
	})
	phase("churn", churn, func(pctx context.Context) {
		var wg sync.WaitGroup
		deployLoop(pctx, &wg)
		list(pctx, &wg, 1, 500*time.Millisecond)
		wg.Wait()
	})
	phase("load", load, func(pctx context.Context) {
		var wg sync.WaitGroup
		deployLoop(pctx, &wg)
		list(pctx, &wg, prof.ListClients, 5*time.Millisecond)
		for w := 0; w < prof.WatchClients; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				watch(pctx, api, rec, &events)
			}()
		}
		wg.Wait()
	})
	_, _ = fmt.Fprintln(stderr, "phase teardown")
	scancel()
	swg.Wait()
	smp.sample(ctx)

	rep.DurationSec = time.Since(runStart).Seconds()
	rep.Nodes, rep.Containers, rep.Sessions = len(fleet.NodeIDs()), fleet.Containers(), fleet.Sessions()
	rep.WatchEvents, rep.Deploys = events.Load(), deploys.Load()
	rep.Resources, rep.Endpoints = smp.snapshot(), rec.stats()
	rep.CPErrors = errorLines(logPath)
	judge(rep, prof.Thresholds)
	return rep, nil
}

// watch holds one v2 SSE stream open and counts the events it receives.
func watch(ctx context.Context, api string, rec *recorder, events *atomic.Int64) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/api/v2/apps?watch=1", nil)
	if err != nil {
		return
	}
	c := &http.Client{Jar: nil}
	start := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			rec.add("WATCH /api/v2/apps connect", time.Since(start), err)
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()
	var cerr error
	if resp.StatusCode >= http.StatusBadRequest {
		cerr = fmt.Errorf("status %d", resp.StatusCode)
	}
	rec.add("WATCH /api/v2/apps connect", time.Since(start), cerr)
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data:") {
			events.Add(1)
		}
	}
}

func waitReady(ctx context.Context, c *http.Client, api string, exited <-chan struct{}) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return errors.New("control plane exited during startup")
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api+"/healthz", nil)
		if resp, err := c.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("control plane did not become healthy in 90s")
}

// bootstrap creates the first admin with the setup token and leaves the session in the client's cookie jar.
func bootstrap(ctx context.Context, c *http.Client, api, bin string, env []string) error {
	cmd := exec.CommandContext(ctx, bin, "setup-token", "--plain") //nolint:gosec // operator-chosen binary
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("setup token: %w", err)
	}
	tok := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	body, _ := json.Marshal(map[string]string{"username": adminEmail, "password": adminPassword, "setup_token": tok})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, api+"/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("register: status %d", resp.StatusCode)
	}
	return nil
}

func errorLines(path string) []string {
	data, err := os.ReadFile(path) //nolint:gosec // scratch path
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, "level=ERROR") && len(out) < 20 {
			out = append(out, truncate(l, 300))
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func judge(r *Report, t stressprofile.Thresholds) {
	add := func(name string, limit, actual float64) {
		r.Checks = append(r.Checks, Check{Name: name, Limit: limit, Actual: actual, Pass: actual <= limit})
	}
	var worst95, worst99 float64
	var reqs, errs int
	for _, e := range r.Endpoints {
		reqs += e.Count
		errs += e.Errors
		if !strings.HasPrefix(e.Endpoint, "WATCH") {
			worst95, worst99 = max(worst95, e.P95Ms), max(worst99, e.P99Ms)
		}
	}
	add("worst endpoint p95 ms", t.P95Ms, worst95)
	add("worst endpoint p99 ms", t.P99Ms, worst99)
	add("rss mb", t.MaxRSSMB, r.Resources.MaxRSSMB)
	add("goroutines", float64(t.MaxGoroutines), float64(r.Resources.MaxGoroutines))
	add("sqlite mb", t.MaxSQLiteMB, r.Resources.MaxSQLiteMB)
	add("reconcile pass ms", t.MaxReconcilePassMs, r.Resources.MaxReconcilePassMs)
	rate := 0.0
	if reqs > 0 {
		rate = float64(errs) * 100 / float64(reqs)
	}
	add("error rate pct", t.MaxErrorRatePct, rate)
	r.Pass = true
	for _, c := range r.Checks {
		r.Pass = r.Pass && c.Pass
	}
}
