package hostdiag

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Meminfo holds the /proc/meminfo fields the free report uses, in bytes.
type Meminfo struct {
	Total, Available, SwapTotal, SwapFree uint64
}

// ParseMeminfo reads /proc/meminfo text; values there are in kB.
func ParseMeminfo(text string) (Meminfo, error) {
	var m Meminfo
	fields := map[string]*uint64{"MemTotal": &m.Total, "MemAvailable": &m.Available, "SwapTotal": &m.SwapTotal, "SwapFree": &m.SwapFree}
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		dst, want := fields[key]
		if !ok || !want {
			continue
		}
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			return Meminfo{}, fmt.Errorf("meminfo: empty %s", key)
		}
		n, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return Meminfo{}, fmt.Errorf("meminfo: %s: %w", key, err)
		}
		*dst = n * 1024
		seen[key] = true
	}
	if !seen["MemTotal"] || !seen["MemAvailable"] {
		return Meminfo{}, fmt.Errorf("meminfo: MemTotal or MemAvailable missing")
	}
	return m, nil
}

// FormatFree renders a Meminfo as the free report.
func FormatFree(m Meminfo) string {
	used := m.Total - min(m.Available, m.Total)
	swapUsed := m.SwapTotal - min(m.SwapFree, m.SwapTotal)
	var b strings.Builder
	fmt.Fprintf(&b, "%-6s %10s %10s %10s\n", "", "total", "used", "available")
	fmt.Fprintf(&b, "%-6s %10s %10s %10s\n", "mem", humanBytes(m.Total), humanBytes(used), humanBytes(m.Available))
	fmt.Fprintf(&b, "%-6s %10s %10s %10s\n", "swap", humanBytes(m.SwapTotal), humanBytes(swapUsed), humanBytes(m.SwapFree))
	return b.String()
}

// ParseUptime reads the first field of /proc/uptime as seconds since boot.
func ParseUptime(text string) (time.Duration, error) {
	f := strings.Fields(text)
	if len(f) == 0 {
		return 0, fmt.Errorf("uptime: empty")
	}
	secs, err := strconv.ParseFloat(f[0], 64)
	if err != nil || secs < 0 {
		return 0, fmt.Errorf("uptime: bad value %q", f[0])
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// Loadavg is the parsed /proc/loadavg.
type Loadavg struct {
	One, Five, Fifteen string
	Runnable, Total    int
}

// ParseLoadavg reads "0.10 0.20 0.30 1/234 5678"; the last pid field is ignored.
func ParseLoadavg(text string) (Loadavg, error) {
	f := strings.Fields(text)
	if len(f) < 4 {
		return Loadavg{}, fmt.Errorf("loadavg: too few fields")
	}
	for _, v := range f[:3] {
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return Loadavg{}, fmt.Errorf("loadavg: bad value %q", v)
		}
	}
	run, total, ok := strings.Cut(f[3], "/")
	r, err1 := strconv.Atoi(run)
	t, err2 := strconv.Atoi(total)
	if !ok || err1 != nil || err2 != nil {
		return Loadavg{}, fmt.Errorf("loadavg: bad task field %q", f[3])
	}
	return Loadavg{One: f[0], Five: f[1], Fifteen: f[2], Runnable: r, Total: t}, nil
}

// FormatLoad renders a Loadavg.
func FormatLoad(l Loadavg, cpus int) string {
	return fmt.Sprintf("load average: %s (1m)  %s (5m)  %s (15m)\ntasks: %d runnable of %d\ncpus: %d\n", l.One, l.Five, l.Fifteen, l.Runnable, l.Total, cpus)
}

// ParseOSRelease returns PRETTY_NAME, falling back to NAME VERSION.
func ParseOSRelease(text string) string {
	vals := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		if uq, err := strconv.Unquote(v); err == nil {
			v = uq
		} else {
			v = strings.Trim(v, `"'`)
		}
		vals[k] = v
	}
	if p := vals["PRETTY_NAME"]; p != "" {
		return p
	}
	return strings.TrimSpace(vals["NAME"] + " " + vals["VERSION"])
}

// ParseVMRSS reads VmRSS from /proc/self/status, in bytes.
func ParseVMRSS(text string) (uint64, error) {
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		k, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok || k != "VmRSS" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			break
		}
		n, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("status: VmRSS: %w", err)
		}
		return n * 1024, nil
	}
	return 0, fmt.Errorf("status: VmRSS missing")
}

// DiskUsage is one statfs result, in bytes.
type DiskUsage struct {
	Label, Path        string
	Total, Used, Avail uint64
}

// FormatDF renders disk usage rows; Used% matches df (used over used plus available).
func FormatDF(rows []DiskUsage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s %10s %10s %10s %5s  %s\n", "", "size", "used", "avail", "use%", "path")
	for _, r := range rows {
		pct := "-"
		if r.Used+r.Avail > 0 {
			pct = fmt.Sprintf("%d%%", (r.Used*100+r.Used+r.Avail-1)/(r.Used+r.Avail))
		}
		fmt.Fprintf(&b, "%-10s %10s %10s %10s %5s  %s\n", r.Label, humanBytes(r.Total), humanBytes(r.Used), humanBytes(r.Avail), pct, r.Path)
	}
	return b.String()
}

// ServiceInfo describes this control plane process.
type ServiceInfo struct {
	Version    string
	GoVersion  string
	Started    time.Time
	Now        time.Time
	Goroutines int
	RSS        uint64
	RSSKnown   bool
	Supervised bool
}

// FormatService renders the service report.
func FormatService(s ServiceInfo) string {
	rss := "unknown"
	if s.RSSKnown {
		rss = humanBytes(s.RSS)
	}
	sup := "not detected (a restart from the dashboard would stop the process)"
	if s.Supervised {
		sup = "detected"
	}
	version := s.Version
	if version == "" {
		version = "unknown"
	}
	return fmt.Sprintf("version:    %s\ngo:         %s\nstarted:    %s\nuptime:     %s\ngoroutines: %d\nrss:        %s\nsupervisor: %s\n",
		version, s.GoVersion, s.Started.UTC().Format(time.RFC3339), HumanDuration(s.Now.Sub(s.Started)), s.Goroutines, rss, sup)
}

// HumanDuration renders d as "3d 4h 5m", or seconds when under a minute.
func HumanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	days := int(d / (24 * time.Hour))
	hours := int(d % (24 * time.Hour) / time.Hour)
	mins := int(d % time.Hour / time.Minute)
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
