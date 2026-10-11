package hostdiag

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	cases := []struct {
		argv    []string
		want    string
		wantErr string
	}{
		{[]string{"host"}, "help", ""},
		{[]string{"host", "df"}, "df", ""},
		{[]string{"host", "free"}, "free", ""},
		{[]string{"host", "uptime"}, "uptime", ""},
		{[]string{"host", "load"}, "load", ""},
		{[]string{"host", "os"}, "os", ""},
		{[]string{"host", "service"}, "service", ""},
		{[]string{"host", "help"}, "help", ""},
		{[]string{"host", "df", "/etc"}, "", "takes no arguments"},
		{[]string{"host", "service", "--json"}, "", "takes no arguments"},
		{[]string{"host", "ps"}, "", "unknown host subcommand"},
		{[]string{"host", "cat"}, "", "unknown host subcommand"},
		{[]string{"host", "/proc/self/environ"}, "", "unknown host subcommand"},
		{[]string{"host", "DF"}, "", "unknown host subcommand"},
		{[]string{"apps", "list"}, "", "not a host command"},
	}
	for _, c := range cases {
		got, err := Check(c.argv)
		if c.wantErr != "" {
			var u *UsageError
			if err == nil || !errors.As(err, &u) || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("Check(%q) err = %v, want %q", c.argv, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("Check(%q) = %q, %v; want %q", c.argv, got, err, c.want)
		}
	}
}

func TestRunHelpAndPlatform(t *testing.T) {
	out, err := Run("help", Info{})
	if err != nil || !strings.Contains(out, "host df") {
		t.Fatalf("help = %q, %v", out, err)
	}
	if _, err := Run("ps", Info{}); err == nil {
		t.Fatal("Run accepted a subcommand outside the list")
	}
	_, err = Run("free", Info{})
	if runtime.GOOS != "linux" {
		if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "not supported on this platform") {
			t.Fatalf("non-linux free err = %v", err)
		}
		return
	}
	for _, sub := range []string{"df", "free", "uptime", "load", "os", "service"} {
		out, err := Run(sub, Info{DataDir: t.TempDir(), Version: "v1"})
		if err != nil || out == "" {
			t.Errorf("Run(%s) = %q, %v", sub, out, err)
		}
	}
}

func TestEnabled(t *testing.T) {
	for v, want := range map[string]bool{"": true, "on": true, "off": false, "OFF": false, "false": false, "0": false, "disabled": false} {
		t.Setenv(EnvSwitch, v)
		if got := Enabled(); got != want {
			t.Errorf("Enabled with %q = %v, want %v", v, got, want)
		}
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"plain\n":                 "plain\n",
		"a\tb":                    "a\tb",
		"\x1b[31mred\x1b[0m":      "[31mred[0m",
		"bell\x07 cr\r nul\x00":   "bell cr nul",
		"c1\u009b2J":              "c12J",
		"bad\xffutf8":             "bad?utf8",
		"\x1b]0;title\x07visible": "]0;titlevisible",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

const meminfoFixture = `MemTotal:        8048836 kB
MemFree:          512000 kB
MemAvailable:    4024418 kB
Buffers:          100000 kB
SwapTotal:       2097148 kB
SwapFree:        1048574 kB
`

func TestParseMeminfo(t *testing.T) {
	m, err := ParseMeminfo(meminfoFixture)
	if err != nil {
		t.Fatal(err)
	}
	if m.Total != 8048836*1024 || m.Available != 4024418*1024 || m.SwapTotal != 2097148*1024 || m.SwapFree != 1048574*1024 {
		t.Fatalf("meminfo = %+v", m)
	}
	out := FormatFree(m)
	if !strings.Contains(out, "mem") || !strings.Contains(out, "7.7GiB") || !strings.Contains(out, "swap") {
		t.Fatalf("free = %q", out)
	}
	for _, bad := range []string{"", "MemFree: 1 kB\n", "MemTotal: x kB\nMemAvailable: 1 kB\n", "MemTotal: 10 kB\n"} {
		if _, err := ParseMeminfo(bad); err == nil {
			t.Errorf("ParseMeminfo(%q) accepted", bad)
		}
	}
}

func TestParseUptimeAndLoad(t *testing.T) {
	d, err := ParseUptime("356012.53 1400000.10\n")
	if err != nil || d.Truncate(time.Second) != 356012*time.Second {
		t.Fatalf("uptime = %v, %v", d, err)
	}
	if HumanDuration(d) != "4d 2h 53m" {
		t.Fatalf("human = %q", HumanDuration(d))
	}
	for _, bad := range []string{"", "abc 1", "-5 1"} {
		if _, err := ParseUptime(bad); err == nil {
			t.Errorf("ParseUptime(%q) accepted", bad)
		}
	}
	l, err := ParseLoadavg("0.52 0.58 0.59 3/412 123456\n")
	if err != nil || l.One != "0.52" || l.Fifteen != "0.59" || l.Runnable != 3 || l.Total != 412 {
		t.Fatalf("loadavg = %+v, %v", l, err)
	}
	if out := FormatLoad(l, 4); !strings.Contains(out, "0.52 (1m)") || strings.Contains(out, "123456") {
		t.Fatalf("load = %q", out)
	}
	for _, bad := range []string{"", "1 2 3", "a 2 3 1/2 9", "1 2 3 x/2 9"} {
		if _, err := ParseLoadavg(bad); err == nil {
			t.Errorf("ParseLoadavg(%q) accepted", bad)
		}
	}
}

func TestParseOSReleaseAndRSS(t *testing.T) {
	cases := map[string]string{
		"NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n": "Ubuntu 24.04.1 LTS",
		"# comment\nNAME=Debian\nVERSION='12 (bookworm)'\n":     "Debian 12 (bookworm)",
		"": "",
	}
	for in, want := range cases {
		if got := ParseOSRelease(in); got != want {
			t.Errorf("ParseOSRelease(%q) = %q, want %q", in, got, want)
		}
	}
	rss, err := ParseVMRSS("Name:\tlevel\nVmRSS:\t   51200 kB\nThreads: 9\n")
	if err != nil || rss != 51200*1024 {
		t.Fatalf("rss = %d, %v", rss, err)
	}
	if _, err := ParseVMRSS("Name: x\n"); err == nil {
		t.Fatal("missing VmRSS accepted")
	}
}

func TestFormatDFAndService(t *testing.T) {
	out := FormatDF([]DiskUsage{{Label: "root", Path: "/", Total: 100 << 30, Used: 40 << 30, Avail: 60 << 30}})
	if !strings.Contains(out, "40%") || !strings.Contains(out, "100.0GiB") {
		t.Fatalf("df = %q", out)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	svc := FormatService(ServiceInfo{Version: "v0.7.0", Started: start, Now: start.Add(90 * time.Minute), Goroutines: 42, RSS: 1 << 20, RSSKnown: true, Supervised: true})
	for _, want := range []string{"v0.7.0", "2026-10-01T00:00:00Z", "1h 30m", "42", "1.0MiB", "detected"} {
		if !strings.Contains(svc, want) {
			t.Errorf("service missing %q: %q", want, svc)
		}
	}
}

// The package must stay unable to run programs, look up users or open sockets.
func TestNoForbiddenImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v", err)
	}
	banned := map[string]bool{"os/exec": true, "os/user": true, "net": true, "unsafe": true, "plugin": true}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // package-local source file from Glob
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if banned[path] || strings.HasPrefix(path, "net/") {
				t.Errorf("%s imports %s", f, path)
			}
		}
	}
}
