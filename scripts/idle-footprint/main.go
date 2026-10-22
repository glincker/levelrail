// Command idle-footprint measures the idle footprint of the control plane on Linux and guards it against a committed baseline.
//
//	idle-footprint measure   boots the binary in IDLE_BIN once per IDLE_COUNTS entry and prints a JSON array
//	idle-footprint check     compares a result file with a baseline file, tolerances come from IDLE_TOL_* env
//	idle-footprint record    merges a result file into a baseline file
//
// Run it through scripts/idle-footprint.sh, which does the container work.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	modeStopped = "stopped"
	modeRunning = "running"
)

// Result is one measured idle point: a fresh control plane holding Apps managed apps.
type Result struct {
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	Apps          int     `json:"apps"`
	AppMode       string  `json:"app_mode"`
	WindowSeconds int     `json:"window_seconds"`
	RSSMB         float64 `json:"rss_mb"`
	PeakRSSMB     float64 `json:"peak_rss_mb"`
	CPUPercent    float64 `json:"cpu_percent"`
	Goroutines    int     `json:"goroutines"`
	OpenFDs       int     `json:"open_fds"`
	Threads       int     `json:"threads"`
	Loops         int     `json:"loops"`
}

func runtimeOS() string   { return runtime.GOOS }
func runtimeArch() string { return runtime.GOARCH }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "idle-footprint:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: idle-footprint measure|check|record")
	}
	switch args[0] {
	case "measure":
		return cmdMeasure()
	case "check":
		return cmdCheck(args[1:])
	case "record":
		return cmdRecord(args[1:])
	}
	return fmt.Errorf("unknown subcommand %q", args[0])
}

func envInt(name string, def int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", name, raw)
	}
	return n, nil
}

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func parseCounts(s string) ([]int, error) {
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("IDLE_COUNTS entry %q is not a non-negative integer", f)
		}
		out = append(out, n)
	}
	return out, nil
}

func cmdMeasure() error {
	counts, err := parseCounts(envOr("IDLE_COUNTS", "0,5,20"))
	if err != nil {
		return err
	}
	window, err := envInt("IDLE_WINDOW_SECONDS", 60)
	if err != nil {
		return err
	}
	settle, err := envInt("IDLE_SETTLE_SECONDS", 20)
	if err != nil {
		return err
	}
	port, err := envInt("IDLE_APP_PORT", 80)
	if err != nil {
		return err
	}
	work := os.TempDir()
	cfg := measureConfig{
		bin: os.Getenv("IDLE_BIN"), counts: counts, appMode: envOr("IDLE_APP_MODE", modeStopped),
		settle: time.Duration(settle) * time.Second, window: time.Duration(window) * time.Second,
		brandFile: envOr("IDLE_BRAND_FILE", "brand.yaml"), fixtures: envOr("IDLE_FIXTURES_FILE", "dev-fixtures.yml"),
		appImage: envOr("IDLE_APP_IMAGE", "traefik/whoami:v1.10.3"), appPort: port,
		workDir: work, httpListen: envOr("IDLE_HTTP_ADDR", "127.0.0.1:18080"),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := measureAll(ctx, cfg)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

func cmdCheck(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: idle-footprint check <baseline.json> <result.json>")
	}
	tol, err := ToleranceFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	return checkFiles(args[0], args[1], tol)
}

func cmdRecord(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: idle-footprint record <baseline.json> <result.json>")
	}
	base, err := readResults(args[0])
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	got, err := readResults(args[1])
	if err != nil {
		return err
	}
	return writeResults(args[0], mergeResults(base, got))
}

func readResults(path string) ([]Result, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator supplied path
	if err != nil {
		return nil, err
	}
	var r []Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return r, nil
}

func mergeResults(base, got []Result) []Result {
	byKey := map[string]Result{}
	for _, r := range base {
		byKey[key(r)] = r
	}
	for _, r := range got {
		byKey[key(r)] = r
	}
	out := make([]Result, 0, len(byKey))
	for _, r := range byKey {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return key(out[i]) < key(out[j]) })
	return out
}

func writeResults(path string, r []Result) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600) //nolint:gosec // operator supplied path
}
