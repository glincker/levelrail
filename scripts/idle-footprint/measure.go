package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	devToken   = "dev-root-token" //nolint:gosec // public dev-mode fixture token
	clockTicks = 100
	readyLimit = 60 * time.Second
)

type measureConfig struct {
	bin        string
	counts     []int
	appMode    string
	settle     time.Duration
	window     time.Duration
	brandFile  string
	fixtures   string
	appImage   string
	appPort    int
	workDir    string
	httpListen string
}

func (c measureConfig) validate() error {
	if c.bin == "" {
		return errors.New("IDLE_BIN is required")
	}
	if c.appMode != modeStopped && c.appMode != modeRunning {
		return fmt.Errorf("IDLE_APP_MODE must be %q or %q, got %q", modeStopped, modeRunning, c.appMode)
	}
	if c.window <= 0 || c.settle < 0 {
		return errors.New("IDLE_WINDOW_SECONDS must be positive and IDLE_SETTLE_SECONDS non-negative")
	}
	if len(c.counts) == 0 {
		return errors.New("IDLE_COUNTS lists no app counts")
	}
	return nil
}

func measureAll(ctx context.Context, c measureConfig) ([]Result, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(c.counts))
	for _, n := range c.counts {
		r, err := measureOne(ctx, c, n)
		if err != nil {
			return nil, fmt.Errorf("apps=%d: %w", n, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func measureOne(ctx context.Context, c measureConfig, apps int) (Result, error) {
	dataDir, err := os.MkdirTemp(c.workDir, "idle-data-")
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(dataDir) }()

	var logBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, c.bin) //nolint:gosec // operator supplied binary under test
	cmd.Env = append(os.Environ(),
		"APP_DEV_MODE=1", "APP_DEV_FIXTURES_FILE="+c.fixtures, "APP_BRAND_FILE="+c.brandFile,
		"APP_DATA_DIR="+dataDir, "APP_HTTP_ADDR="+c.httpListen, "APP_AGENT_ADDR=127.0.0.1:0",
		"APP_INGRESS_HTTP_ADDR=127.0.0.1:0", "APP_INGRESS_HTTPS_ADDR=127.0.0.1:0",
		"APP_API_RATE_LIMIT_READ_RPM=10000000", "APP_API_RATE_LIMIT_WRITE_RPM=10000000",
	)
	cmd.Stdout, cmd.Stderr = &logBuf, &logBuf
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start control plane: %w", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	api := &apiClient{base: "http://" + c.httpListen, http: &http.Client{Timeout: 30 * time.Second}}
	pid := cmd.Process.Pid
	if err := api.waitReady(ctx, pid, readyLimit); err != nil {
		return Result{}, fmt.Errorf("%w\n%s", err, tail(logBuf.String(), 2000))
	}
	for i := 1; i <= apps; i++ {
		if err := api.createApp(ctx, fmt.Sprintf("idle-%04d", i), c); err != nil {
			return Result{}, err
		}
	}
	if c.appMode == modeRunning {
		defer api.deleteApps(apps)
		if err := api.waitRunning(ctx, apps, readyLimit*3); err != nil {
			return Result{}, err
		}
	}
	if err := sleepCtx(ctx, c.settle); err != nil {
		return Result{}, err
	}

	before, _ := api.health(ctx)
	cpu0, err := procCPUSeconds(pid)
	if err != nil {
		return Result{}, err
	}
	began := time.Now()
	var peakRSS uint64
	for time.Since(began) < c.window {
		if err := sleepCtx(ctx, time.Second); err != nil {
			return Result{}, err
		}
		if rss, _, err := procStatus(pid); err == nil && rss > peakRSS {
			peakRSS = rss
		}
	}
	elapsed := time.Since(began).Seconds()
	cpu1, err := procCPUSeconds(pid)
	if err != nil {
		return Result{}, err
	}
	rss, threads, err := procStatus(pid)
	if err != nil {
		return Result{}, err
	}
	fds, err := countFDs(pid)
	if err != nil {
		return Result{}, err
	}
	health, err := api.health(ctx)
	if err != nil {
		return Result{}, err
	}
	if os.Getenv("IDLE_LOOPS_DUMP") != "" {
		dumpLoops(apps, before.Footprint.Loops, health.Footprint.Loops)
	}
	return Result{
		OS: runtimeOS(), Arch: runtimeArch(), Apps: apps, AppMode: c.appMode, WindowSeconds: int(c.window / time.Second),
		RSSMB: float64(rss) / (1 << 20), PeakRSSMB: float64(max(peakRSS, rss)) / (1 << 20), CPUPercent: (cpu1 - cpu0) / elapsed * 100,
		Goroutines: health.Runtime.Goroutines, OpenFDs: fds, Threads: threads, Loops: len(health.Footprint.Loops),
	}, nil
}

type apiClient struct {
	base string
	http *http.Client
}

type loopInfo struct {
	Name   string  `json:"name"`
	Runs   int64   `json:"runs"`
	BusyMS float64 `json:"busy_ms_total"`
}

type healthBody struct {
	Runtime struct {
		Goroutines int `json:"goroutines"`
	} `json:"runtime"`
	Footprint struct {
		Loops []loopInfo `json:"loops"`
	} `json:"footprint"`
}

func (a *apiClient) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+devToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, tail(string(b), 300))
	}
	return b, nil
}

func (a *apiClient) waitReady(ctx context.Context, pid int, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); err != nil {
			return errors.New("control plane exited during boot")
		}
		if _, err := a.do(ctx, http.MethodGet, "/api/v1/apps", nil); err == nil {
			return nil
		}
		if err := sleepCtx(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
	return errors.New("control plane did not become ready")
}

func (a *apiClient) createApp(ctx context.Context, name string, c measureConfig) error {
	image := c.appImage
	if c.appMode == modeStopped {
		image = "idle.invalid/none:0"
	}
	body, _ := json.Marshal(map[string]any{"name": name, "image": image, "port": c.appPort})
	if _, err := a.do(ctx, http.MethodPost, "/api/v1/apps", body); err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	action := "start"
	if c.appMode == modeStopped {
		action = "stop"
	}
	if _, err := a.do(ctx, http.MethodPost, "/api/v1/apps/"+name+"/"+action, nil); err != nil {
		return fmt.Errorf("%s %s: %w", action, name, err)
	}
	return nil
}

func (a *apiClient) waitRunning(ctx context.Context, want int, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	got := -1
	for time.Now().Before(deadline) {
		if b, err := a.do(ctx, http.MethodGet, "/api/v1/apps-summary", nil); err == nil {
			var s struct {
				Running int `json:"running"`
			}
			if json.Unmarshal(b, &s) == nil {
				if got = s.Running; got >= want {
					return nil
				}
			}
		}
		if err := sleepCtx(ctx, time.Second); err != nil {
			return err
		}
	}
	return fmt.Errorf("only %d of %d apps reached running", got, want)
}

// deleteApps removes the containers the run started on the host Docker daemon.
func (a *apiClient) deleteApps(n int) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for i := 1; i <= n; i++ {
		_, _ = a.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/apps/idle-%04d", i), nil)
	}
}

func (a *apiClient) health(ctx context.Context) (healthBody, error) {
	b, err := a.do(ctx, http.MethodGet, "/api/v1/system/control-plane-health", nil)
	if err != nil {
		return healthBody{}, err
	}
	var h healthBody
	if err := json.Unmarshal(b, &h); err != nil {
		return healthBody{}, fmt.Errorf("parse control-plane-health: %w", err)
	}
	return h, nil
}

func procStatus(pid int) (rssBytes uint64, threads int, err error) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		switch k {
		case "VmRSS":
			kb, _ := strconv.ParseUint(f[0], 10, 64)
			rssBytes = kb << 10
		case "Threads":
			threads, _ = strconv.Atoi(f[0])
		}
	}
	if rssBytes == 0 {
		return 0, 0, errors.New("VmRSS not found, the sampler needs Linux /proc")
	}
	return rssBytes, threads, nil
}

func procCPUSeconds(pid int) (float64, error) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, errors.New("malformed /proc stat")
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 13 {
		return 0, errors.New("short /proc stat")
	}
	ut, _ := strconv.ParseFloat(f[11], 64)
	st, _ := strconv.ParseFloat(f[12], 64)
	return (ut + st) / clockTicks, nil
}

func countFDs(pid int) (int, error) {
	e, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fd"))
	if err != nil {
		return 0, err
	}
	return len(e), nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// dumpLoops prints per-loop runs and busy time inside the window, so a loop that grows with the app count shows up.
func dumpLoops(apps int, before, after []loopInfo) {
	prev := map[string]loopInfo{}
	for _, l := range before {
		prev[l.Name] = l
	}
	for _, l := range after {
		p := prev[l.Name]
		if runs := l.Runs - p.Runs; runs > 0 {
			fmt.Fprintf(os.Stderr, "loops apps=%d %-40s runs=%d busy_ms=%.1f\n", apps, l.Name, runs, l.BusyMS-p.BusyMS)
		}
	}
}
