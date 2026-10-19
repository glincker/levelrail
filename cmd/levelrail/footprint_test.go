package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/demo"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
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

// TestFootprintPhase is the child process of TestFootprint: it boots the demo control plane on seeded fixtures, idles, and prints one result line.
func TestFootprintPhase(t *testing.T) {
	mode := os.Getenv(envFootprintPhase)
	if mode == "" {
		t.Skip("child of TestFootprint")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Setenv("APP_DATA_DIR", t.TempDir())
	db, err := openStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tdb, err := openTelemetryStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tdb.Close() }()
	adb, err := openAlertingStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = adb.Close() }()
	seedFootprintDemo(ctx, t, db, tdb, adb)
	if _, err := buildDemoHandler(ctx, demoTestLogger(), demoTestBrand(), db, tdb, adb, telemetry.NewLogBroadcaster()); err != nil {
		t.Fatal(err)
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
}

// TestFootprint measures idle RSS, goroutines, threads and CPU with every experimental feature off and on, and fails on a regression against testdata/footprint_baseline.json. Set FOOTPRINT_RECORD=1 to rewrite the baseline.
func TestFootprint(t *testing.T) {
	if testing.Short() || os.Getenv(envFootprintPhase) != "" {
		t.Skip("footprint harness runs in the nightly lane, not under -short")
	}
	all := make([]string, 0, len(experimental.All()))
	for _, f := range experimental.All() {
		all = append(all, string(f))
	}
	got := footprintBaselineFile{}
	for mode, features := range map[string]string{"off": "", "on": strings.Join(all, ",")} {
		got[mode] = runFootprintPhase(t, mode, features)
		t.Logf("%s: %+v", mode, got[mode])
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

func runFootprintPhase(t *testing.T, mode, features string) footprintResult {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestFootprintPhase$", "-test.v", "-test.timeout", "10m") //nolint:gosec // re-runs this test binary
	cmd.Env = append(os.Environ(), envFootprintPhase+"="+mode, experimental.EnvVar+"="+features)
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

func seedFootprintDemo(ctx context.Context, t *testing.T, db *store.DB, tdb *telemetry.DB, adb *alerting.DB) {
	t.Helper()
	anchor := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	if _, err := demo.SeedAll(ctx, demo.Stores{DB: db, Telemetry: tdb, Alerting: adb}, demo.Options{Seed: 1, Anchor: anchor}); err != nil {
		t.Fatal(err)
	}
}
