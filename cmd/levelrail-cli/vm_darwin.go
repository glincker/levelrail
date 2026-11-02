//go:build darwin

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/localmode"
	"github.com/GLINCKER/levelrail/internal/vmruntime"
)

const bytesPerMiB = 1 << 20

type limactlRunner struct {
	bin  string
	home string
}

func (r limactlRunner) cmd(ctx context.Context, args []string) *exec.Cmd {
	c := exec.CommandContext(ctx, r.bin, args...) //nolint:gosec // limactl from our data dir or PATH, fixed argv
	// A private Lima home keeps our instance apart from the user's own.
	c.Env = append(os.Environ(), "LIMA_HOME="+r.home)
	return c
}

func (r limactlRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	var out, errb bytes.Buffer
	c := r.cmd(ctx, args)
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

func (r limactlRunner) Interactive(ctx context.Context, args ...string) error {
	c := r.cmd(ctx, args)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Stream copies stdout straight to w so a large export never sits in memory.
func (r limactlRunner) Stream(ctx context.Context, w io.Writer, args ...string) error {
	var errb bytes.Buffer
	c := r.cmd(ctx, args)
	c.Stdout, c.Stderr = w, &errb
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return fmt.Errorf("limactl %s: %w: %s", args[0], err, msg)
	}
	return nil
}

func newVMEnv(lookup func(string) (string, bool)) (*vmEnv, error) {
	settings, err := vmruntime.DefaultPath()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Dir(settings)
	// limactl comes from the data dir (pinned download, later slice) or PATH.
	bin := filepath.Join(dataDir, "bin", "limactl")
	if _, serr := os.Stat(bin); serr != nil {
		if bin, err = exec.LookPath("limactl"); err != nil {
			return nil, errors.New("limactl not found; install Lima (the pinned download lands in a later slice)")
		}
	}
	maxPath, err := vmruntime.SocketPathMax(lookup)
	if err != nil {
		return nil, err
	}
	limaHome, short, err := chooseLimaHome(filepath.Join(dataDir, "lima"), home, vmInstance, maxPath)
	if err != nil {
		return nil, err
	}
	if short {
		if err := ensureLimaHome(limaHome); err != nil {
			return nil, err
		}
	}
	out, err := exec.CommandContext(context.Background(), "sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return nil, fmt.Errorf("read host memory: %w", err)
	}
	mem, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return nil, err
	}
	return &vmEnv{
		runner:       limactlRunner{bin: bin, home: limaHome},
		limaHome:     limaHome,
		limaShort:    short,
		settingsPath: settings,
		dataDir:      dataDir,
		homeDir:      home,
		hostMemMiB:   int(mem / bytesPerMiB),
		hostCPUs:     runtime.NumCPU(),
		arch:         vmHostArch(),
		portFree:     localmode.LoopbackFree,
		probe:        httpProbe,
		sleep:        time.Sleep,
	}, nil
}
