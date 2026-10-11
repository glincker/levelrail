//go:build linux

package hostdiag

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"syscall"
	"time"
)

// Fixed paths only: nothing a request supplies ever reaches a filesystem call.
const (
	procMeminfo    = "/proc/meminfo"
	procUptime     = "/proc/uptime"
	procLoadavg    = "/proc/loadavg"
	procSelfStatus = "/proc/self/status"
	osRelease      = "/etc/os-release"
	osReleaseAlt   = "/usr/lib/os-release"
)

func collect(sub string, info Info) (string, error) {
	switch sub {
	case "df":
		return collectDF(info.DataDir)
	case "free":
		text, err := readSmall(procMeminfo)
		if err != nil {
			return "", err
		}
		m, err := ParseMeminfo(text)
		if err != nil {
			return "", err
		}
		return FormatFree(m), nil
	case "uptime":
		text, err := readSmall(procUptime)
		if err != nil {
			return "", err
		}
		d, err := ParseUptime(text)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("up %s (since %s)\n", HumanDuration(d), time.Now().Add(-d).UTC().Format(time.RFC3339)), nil
	case "load":
		text, err := readSmall(procLoadavg)
		if err != nil {
			return "", err
		}
		l, err := ParseLoadavg(text)
		if err != nil {
			return "", err
		}
		return FormatLoad(l, runtime.NumCPU()), nil
	case "os":
		return collectOS()
	case "service":
		return collectService(info), nil
	}
	return "", &UsageError{Reason: fmt.Sprintf("unknown host subcommand %q", sub)}
}

func collectDF(dataDir string) (string, error) {
	targets := []DiskUsage{{Label: "root", Path: "/"}}
	if dataDir != "" {
		targets = append([]DiskUsage{{Label: "data dir", Path: dataDir}}, targets...)
	}
	rows := make([]DiskUsage, 0, len(targets))
	for _, t := range targets {
		var st syscall.Statfs_t
		if err := syscall.Statfs(t.Path, &st); err != nil {
			return "", fmt.Errorf("statfs %s: %w", t.Label, err)
		}
		bs := uint64(st.Bsize) //nolint:gosec // block size is always positive
		t.Total = st.Blocks * bs
		t.Used = (st.Blocks - st.Bfree) * bs
		t.Avail = st.Bavail * bs
		rows = append(rows, t)
	}
	return FormatDF(rows), nil
}

func collectOS() (string, error) {
	text, err := readSmall(osRelease)
	if err != nil {
		text, err = readSmall(osReleaseAlt)
	}
	name := "unknown"
	if err == nil {
		if n := ParseOSRelease(text); n != "" {
			name = n
		}
	}
	var u syscall.Utsname
	if err := syscall.Uname(&u); err != nil {
		return "", fmt.Errorf("uname: %w", err)
	}
	return fmt.Sprintf("os:     %s\nkernel: %s %s\narch:   %s\n", name, cstr(u.Sysname[:]), cstr(u.Release[:]), cstr(u.Machine[:])), nil
}

func collectService(info Info) string {
	s := ServiceInfo{
		Version: info.Version, GoVersion: runtime.Version(), Started: processStart, Now: time.Now(),
		Goroutines: runtime.NumGoroutine(), Supervised: info.Supervised,
	}
	if text, err := readSmall(procSelfStatus); err == nil {
		if rss, perr := ParseVMRSS(text); perr == nil {
			s.RSS, s.RSSKnown = rss, true
		}
	}
	return FormatService(s)
}

const maxProcRead = 64 << 10

func readSmall(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is one of the constants above
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxProcRead))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}

// cstr converts a NUL-terminated utsname field; its element type differs by arch.
func cstr[T int8 | uint8](b []T) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}
