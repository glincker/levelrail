package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// limaExec runs limactl for the builder instance in the shared Lima home.
type limaExec struct{ bin, home string }

func (l limaExec) Run(ctx context.Context, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, l.bin, args...) //nolint:gosec // limactl found on PATH or in our data dir, fixed argv
	c.Env = append(os.Environ(), "LIMA_HOME="+l.home)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return out.Bytes(), fmt.Errorf("limactl %s: %w: %s", args[0], err, msg)
	}
	return out.Bytes(), nil
}

func builderDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Levelrail", "builder"), nil
}

func lookupLimactl() (string, error) { return exec.LookPath("limactl") }

func hostMemMiB() int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.CommandContext(context.Background(), "sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return int(n >> 20)
	case "linux":
		b, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}
		for _, l := range strings.Split(string(b), "\n") {
			if f := strings.Fields(l); len(f) >= 2 && f[0] == "MemTotal:" {
				kb, _ := strconv.Atoi(f[1])
				return kb / 1024
			}
		}
	}
	return 0
}

// onBattery reads the power source with pmset; other systems report mains power.
func onBattery(ctx context.Context) (bool, error) {
	if runtime.GOOS != "darwin" {
		return false, nil
	}
	out, err := exec.CommandContext(ctx, "pmset", "-g", "batt").Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), "'Battery Power'"), nil
}

// serverTime does an outbound TLS request and returns the server's Date header.
func serverTime(timeout time.Duration) func(context.Context, string) (time.Time, error) {
	return func(ctx context.Context, server string) (time.Time, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, server, nil)
		if err != nil {
			return time.Time{}, err
		}
		c := http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
		resp, err := c.Do(req)
		if err != nil {
			return time.Time{}, fmt.Errorf("cannot reach %s: %w", server, err)
		}
		_ = resp.Body.Close()
		t, err := http.ParseTime(resp.Header.Get("Date"))
		if err != nil {
			return time.Time{}, errors.New("the control plane sent no usable Date header")
		}
		return t, nil
	}
}
