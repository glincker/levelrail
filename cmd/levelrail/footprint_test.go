package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/profile"
)

const (
	envFootprintPhase  = "FOOTPRINT_PHASE"
	envFootprintIdle   = "APP_FOOTPRINT_IDLE_SECONDS"
	envFootprintRecord = "FOOTPRINT_RECORD"
	footprintResultTag = "FOOTPRINT_RESULT "
	footprintBaseline  = "testdata/footprint_baseline.json"
	rssGrowthLimit     = 1.15
	goroutineSlack     = 5
	defaultIdleSeconds = 60
	bootSettle         = 20 * time.Second
	footprintBrandFile = "../../brand.yaml"
)

type footprintResult struct {
	Mode       string  `json:"mode"`
	RSSMB      float64 `json:"rss_mb"`
	Goroutines int     `json:"goroutines"`
	Threads    int     `json:"threads"`
	CPUPercent float64 `json:"cpu_percent"`
	Loops      int     `json:"loops"`
}

type footprintBaselineFile map[string]footprintResult

func idleSeconds() int {
	if n, err := strconv.Atoi(os.Getenv(envFootprintIdle)); err == nil && n > 0 {
		return n
	}
	return defaultIdleSeconds
}

// TestFootprintPhase is the child process of TestFootprint: it boots the real control plane (run) on a temp data dir, with no Docker daemon needed,, idles, and prints one result line.
func TestFootprintPhase(t *testing.T) {
	mode := os.Getenv(envFootprintPhase)
	if mode == "" {
		t.Skip("child of TestFootprint")
	}
	t.Setenv("APP_DATA_DIR", t.TempDir())
	t.Setenv("APP_BRAND_FILE", footprintBrandFile)
	t.Setenv("APP_HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("APP_INGRESS_HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("APP_INGRESS_HTTPS_ADDR", "127.0.0.1:0")
	t.Setenv("APP_AGENT_ADDR", "127.0.0.1:0")
	runErr := make(chan error, 1)
	go func() { runErr <- run(slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	select {
	case err := <-runErr:
		t.Fatalf("control plane exited during boot: %v", err)
	case <-time.After(bootSettle):
	}

	idle := time.Duration(idleSeconds()) * time.Second
	time.Sleep(idle / 4)
	start := footprint.ReadProc()
	began := time.Now()
	var peak footprint.Proc
	for time.Since(began) < idle {
		time.Sleep(time.Second)
		if p := footprint.ReadProc(); p.RSSBytes > peak.RSSBytes {
			peak = p
		}
	}
	end := footprint.ReadProc()
	res := footprintResult{
		Mode: mode, RSSMB: float64(peak.RSSBytes) / (1 << 20), Goroutines: end.Goroutines, Threads: end.Threads,
		CPUPercent: (end.CPUSeconds - start.CPUSeconds) / time.Since(began).Seconds() * 100, Loops: len(footprint.Loops()),
	}
	b, _ := json.Marshal(res)
	fmt.Println(footprintResultTag + string(b))
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	select {
	case <-runErr:
	case <-time.After(30 * time.Second):
	}
}

// TestFootprint measures idle RSS, goroutines, threads and CPU with every experimental feature off and on and under each server profile, and fails on a regression against testdata/footprint_baseline.json. Set FOOTPRINT_RECORD=1 to rewrite the baseline.
func TestFootprint(t *testing.T) {
	if testing.Short() || os.Getenv(envFootprintPhase) != "" {
		t.Skip("footprint harness runs in the nightly lane, not under -short")
	}
	all := make([]string, 0, len(experimental.All()))
	for _, f := range experimental.All() {
		all = append(all, string(f))
	}
	got := footprintBaselineFile{}
	phases := []footprintPhase{
		{mode: "off"},
		{mode: "on", features: strings.Join(all, ",")},
		{mode: "profile-minimal", profile: profile.Minimal},
		{mode: "profile-standard", profile: profile.Standard},
		{mode: "profile-suite", profile: profile.Suite},
	}
	for _, ph := range phases {
		got[ph.mode] = runFootprintPhase(t, ph)
		t.Logf("%s: %+v", ph.mode, got[ph.mode])
	}
	lo, std := got["profile-minimal"], got["profile-standard"]
	if lo.Goroutines >= std.Goroutines || lo.Loops >= std.Loops {
		t.Errorf("minimal must start less than standard: goroutines %d vs %d, loops %d vs %d", lo.Goroutines, std.Goroutines, lo.Loops, std.Loops)
	}

	key := func(mode string) string { return runtime.GOOS + "/" + mode }
	baseline := footprintBaselineFile{}
	if b, err := os.ReadFile(footprintBaseline); err == nil {
		if err := json.Unmarshal(b, &baseline); err != nil {
			t.Fatalf("parse baseline: %v", err)
		}
	}
	if os.Getenv(envFootprintRecord) != "" {
		for mode, r := range got {
			baseline[key(mode)] = r
		}
		b, err := json.MarshalIndent(baseline, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(footprintBaseline), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(footprintBaseline, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	for mode, r := range got {
		want, ok := baseline[key(mode)]
		if !ok {
			t.Logf("no baseline for %s, run with %s=1 to record it", key(mode), envFootprintRecord)
			continue
		}
		if r.Goroutines > want.Goroutines+goroutineSlack {
			t.Errorf("%s: idle goroutines %d exceed ratchet %d (+%d slack)", mode, r.Goroutines, want.Goroutines, goroutineSlack)
		}
		if r.RSSMB > want.RSSMB*rssGrowthLimit {
			t.Errorf("%s: idle RSS %.1f MB is more than 15 percent over baseline %.1f MB", mode, r.RSSMB, want.RSSMB)
		}
	}
}

type footprintPhase struct {
	mode     string
	features string
	profile  profile.Profile
}

func runFootprintPhase(t *testing.T, ph footprintPhase) footprintResult {
	t.Helper()
	mode, features := ph.mode, ph.features
	cmd := exec.Command(os.Args[0], "-test.run", "^TestFootprintPhase$", "-test.v", "-test.timeout", "10m") //nolint:gosec // re-runs this test binary
	cmd.Env = append(os.Environ(), envFootprintPhase+"="+mode, experimental.EnvVar+"="+features, profile.EnvVar+"="+string(ph.profile))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s phase failed: %v\n%s", mode, err, out.String())
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, footprintResultTag); ok {
			var r footprintResult
			if err := json.Unmarshal([]byte(rest), &r); err != nil {
				t.Fatalf("parse result: %v", err)
			}
			return r
		}
	}
	t.Fatalf("%s phase printed no result:\n%s", mode, out.String())
	return footprintResult{}
}
