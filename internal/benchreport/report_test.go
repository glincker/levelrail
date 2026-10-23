package benchreport

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

func sampleResults() []chaos.Result {
	a := chaos.Result{Scenario: chaos.DeployLatency, Status: chaos.StatusPassed, Summary: "cold 4900 ms | warm", RanAt: time.Unix(0, 0)}
	a.Set("deploy_cold_ms", 4900)
	a.Set("deploy_warm_p50_ms", 4260.5)
	b := a
	b.Metrics = chaos.Metrics{"deploy_cold_ms": 5100}
	k := chaos.Result{Scenario: chaos.KillContainer, Status: chaos.StatusFailed, RecoveryMS: 3525, ErrorWindowMS: 3500, Requests: 1200, RequestErrors: 141}
	return []chaos.Result{a, b, k}
}

func mac() Host {
	return Host{OS: "darwin", Arch: "arm64", CPUs: 14, CPUModel: "Apple M4 Max", MemBytes: 36 << 30, LoadStart: 30, LoadEnd: 12,
		Docker: Docker{Version: "28.3.0", OS: "Docker Desktop", MemBytes: 7 << 30}}
}

func TestCaveats(t *testing.T) {
	tests := []struct {
		name  string
		host  Host
		build Build
		rep   int
		ratio float64
		want  []string
		deny  []string
	}{
		{"mac laptop", mac(), Build{Commit: "abc"}, 1, 1, []string{"NOT Linux production", "Docker Desktop", "busy", "Single run"}, nil},
		{"quiet linux repeated", Host{OS: "linux", CPUs: 8, LoadStart: 0.1, LoadEnd: 0.2}, Build{Commit: "abc"}, 5, 1, []string{"share one machine"}, []string{"NOT Linux", "busy", "Single run"}},
		{"ratio unset never flags busy", mac(), Build{Commit: "abc"}, 3, 0, []string{"load average"}, []string{"busy"}},
		{"unstamped dirty build", Host{OS: "linux", CPUs: 2, LoadStart: -1}, Build{Commit: "unknown", Dirty: true}, 2, 1, []string{"not tied to a commit", "uncommitted"}, []string{"load average"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(Caveats(tt.host, tt.build, tt.rep, tt.ratio), "\n")
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, d := range tt.deny {
				if strings.Contains(got, d) {
					t.Errorf("unexpected %q in:\n%s", d, got)
				}
			}
		})
	}
}

func TestLimitsReflectWhatRan(t *testing.T) {
	status := map[string]string{}
	for _, l := range Limits(sampleResults(), 2) {
		status[l.Question] = l.Status
	}
	if got := status["Deploy latency from image to serving"]; !strings.Contains(got, "2 run(s)") {
		t.Errorf("deploy status = %q", got)
	}
	if got := status["Kill the serving container under traffic"]; !strings.Contains(got, "failed an assertion") {
		t.Errorf("kill status = %q", got)
	}
	if got := status["Rising steady load through the ingress"]; got != "not run" {
		t.Errorf("steady status = %q", got)
	}
	if got := status["Statistical confidence"]; !strings.Contains(got, "2 run(s)") {
		t.Errorf("confidence = %q", got)
	}
	if _, ok := status["Anything on a Linux server, a real network or a real application"]; !ok {
		t.Error("the fixed not-measured rows must always be present")
	}
	for _, l := range Limits(sampleResults(), 5) {
		if l.Question == "Statistical confidence" {
			t.Error("confidence row should disappear at 5 runs")
		}
	}
}

func TestValidate(t *testing.T) {
	good := New(sampleResults(), Options{Build: Build{Version: "v1", Commit: "abc"}, Host: mac(), Repeat: 2, Now: time.Now()})
	if err := good.Validate(); err != nil {
		t.Fatalf("good report rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Report)
		want   string
	}{
		{"no results", func(r *Report) { r.Results = nil }, "results"},
		{"no commit", func(r *Report) { r.Build.Commit = "" }, "build commit"},
		{"no version", func(r *Report) { r.Build.Version = "" }, "build version"},
		{"no host", func(r *Report) { r.Host = Host{} }, "host"},
		{"no method", func(r *Report) { r.Methodology = nil }, "methodology"},
		{"no time", func(r *Report) { r.GeneratedAt = time.Time{} }, "timestamp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := good
			tt.mutate(&r)
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want mention of %q", err, tt.want)
			}
		})
	}
}

func TestMarkdown(t *testing.T) {
	r := New(sampleResults(), Options{Build: Build{Version: "v0.8.3", Commit: "abc123", Dirty: true}, Host: mac(), Repeat: 2, Now: time.Date(2026, 10, 22, 13, 0, 0, 0, time.UTC)})
	md := Markdown(r)
	for _, want := range []string{
		"# Benchmark results", "| Version | v0.8.3 |", "abc123 (dirty tree)", "Apple M4 Max", "Docker 28.3.0 (Docker Desktop)",
		"| Load average at start and end | 30.00 and 12.00 |", "## Read this first", "NOT Linux production",
		"| deploy-latency | 2 | passed |", "| kill-container | 1 | failed | 3525 ms | 141 of 1200 | 3500 ms |",
		"cold 4900 ms / warm", "### deploy-latency", "| Metric | Run 1 | Run 2 |", "| deploy_warm_p50_ms | 4260.50 | n/a |", "| deploy_cold_ms | 4900 | 5100 |",
		"## Limits table", "## Method", "open loop",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
	if strings.ContainsAny(md, string([]rune{0x2013, 0x2014})) {
		t.Error("markdown must not contain en or em dashes")
	}
	if strings.Contains(md, "### kill-container") {
		t.Error("a scenario without metrics should not get a measurements table")
	}
}

func TestWriteFilesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := New(sampleResults(), Options{Build: Build{Version: "v1", Commit: "abc"}, Host: mac(), Repeat: 2, Now: time.Now()})
	jp, mp := filepath.Join(dir, "r.json"), filepath.Join(dir, "r.md")
	if err := WriteJSON(jp, r); err != nil {
		t.Fatal(err)
	}
	if err := WriteMarkdown(mp, r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(jp) //nolint:gosec // path is under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.SchemaVersion != SchemaVersion || back.Build.Commit != "abc" || back.Host.CPUModel != "Apple M4 Max" || len(back.Results) != 3 || back.Results[0].Metrics["deploy_cold_ms"] != 4900 {
		t.Errorf("round trip lost data: %+v", back)
	}
	if err := back.Validate(); err != nil {
		t.Errorf("round tripped report invalid: %v", err)
	}
	if err := WriteJSON(filepath.Join(dir, "missing", "r.json"), r); err == nil {
		t.Error("writing into a missing directory should fail")
	}
}

func TestDetectHost(t *testing.T) {
	cpuinfo := "processor\t: 0\nmodel name\t: AMD EPYC 7B13\n"
	files := map[string]string{
		"/proc/cpuinfo":              cpuinfo,
		"/proc/meminfo":              "MemTotal:       16384000 kB\nMemFree: 1 kB\n",
		"/etc/os-release":            "NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n",
		"/proc/sys/kernel/osrelease": "6.8.0-31-generic\n",
		"/proc/loadavg":              "0.52 0.40 0.30 1/200 999\n",
	}
	p := Probe{GOOS: "linux", GOARCH: "amd64", NumCPU: 8, ReadFile: func(path string) ([]byte, error) {
		if s, ok := files[path]; ok {
			return []byte(s), nil
		}
		return nil, errors.New("absent")
	}}
	h := DetectHost(context.Background(), p)
	if h.CPUModel != "AMD EPYC 7B13" || h.MemBytes != 16384000*1024 || h.OSVersion != "Ubuntu 24.04 LTS" || h.Kernel != "6.8.0-31-generic" || h.LoadStart != 0.52 {
		t.Errorf("linux host = %+v", h)
	}

	outs := map[string]string{"sysctl -n machdep.cpu.brand_string": "Apple M4 Max", "sysctl -n hw.memsize": "38654705664", "sw_vers -productVersion": "27.2", "uname -r": "26.0.0", "sysctl -n vm.loadavg": "{ 30.43 23.17 21.13 }"}
	d := Probe{GOOS: "darwin", GOARCH: "arm64", NumCPU: 14, Run: func(_ context.Context, name string, args ...string) (string, error) {
		if v, ok := outs[name+" "+strings.Join(args, " ")]; ok {
			return v, nil
		}
		return "", errors.New("absent")
	}}
	h = DetectHost(context.Background(), d)
	if h.CPUModel != "Apple M4 Max" || h.MemBytes != 38654705664 || h.OSVersion != "27.2" || h.LoadStart != 30.43 {
		t.Errorf("darwin host = %+v", h)
	}
	if !strings.Contains(h.Summary(), "36 GiB RAM") {
		t.Errorf("summary = %q", h.Summary())
	}
}

func TestDetectHostUnreadableLeavesFieldsEmpty(t *testing.T) {
	h := DetectHost(context.Background(), Probe{GOOS: "linux", GOARCH: "arm64", NumCPU: 2, ReadFile: func(string) ([]byte, error) { return nil, errors.New("no") }})
	if h.CPUModel != "" || h.MemBytes != 0 || h.LoadStart != -1 {
		t.Errorf("host = %+v", h)
	}
}

func TestParsers(t *testing.T) {
	if ParseMemTotal("MemTotal: bad kB") != 0 || ParseMemTotal("nothing") != 0 {
		t.Error("bad meminfo must parse to 0")
	}
	if ParseCPUInfoModel("flags : a") != "" || ParseOSRelease("ID=x") != "" {
		t.Error("absent keys must parse to empty")
	}
	if parseFirstFloat("") != -1 || parseFirstFloat("x") != -1 {
		t.Error("bad load must parse to -1")
	}
}
