// Package buildkitd supervises the managed rootless buildkitd container that
// serves source builds on engines without an embedded BuildKit (ADR 046).
package buildkitd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// ContainerName is the single managed daemon per node.
	ContainerName = "levelrail-buildkitd"
	// VolumeName holds the daemon state and local cache.
	VolumeName = "levelrail-buildkit-state"

	// DefaultImageRef tracks the moby/buildkit client in go.mod.
	DefaultImageRef = "docker.io/moby/buildkit:v0.34.1-rootless"
	// DefaultDigest stays empty until verified on a real host, so
	// APP_BUILDKIT_IMAGE must supply a pinned reference until then.
	DefaultDigest = ""

	defaultGCKeepMB       = 10240
	defaultGCKeepDuration = 168 * time.Hour
	defaultHealthTimeout  = 5 * time.Second
	defaultIdleStop       = 30 * time.Minute

	containerSocketDir = "/run/levelrail-buildkit"
	socketName         = "buildkitd.sock"
	sha256Prefix       = "@sha256:"
)

var (
	// ErrImageNotPinned rejects an image reference without a sha256 digest.
	ErrImageNotPinned = errors.New("buildkitd: image must be pinned by sha256 digest (set APP_BUILDKIT_IMAGE)")
	// ErrUnavailable means the daemon is not healthy after one restart.
	ErrUnavailable = errors.New("buildkitd: buildkit_unavailable")
)

// Config holds the supervisor settings, read from APP_BUILDKIT_* env.
type Config struct {
	Image         string
	Snapshotter   string
	GCKeepMB      int64
	GCKeepFor     time.Duration
	HealthTimeout time.Duration
	IdleStop      time.Duration
	SocketDir     string
	ConfigDir     string
}

// SocketAddr is the unix address a BuildKit client dials.
func (c Config) SocketAddr() string {
	return "unix://" + filepath.Join(c.SocketDir, socketName)
}

// ConfigFromEnv reads the settings. Rootless puts the socket under
// XDG_RUNTIME_DIR, rootful under APP_DATA_DIR, as ADR 046 requires.
func ConfigFromEnv(get func(string) string, rootless bool) (Config, error) {
	cfg := Config{
		Image:         DefaultImageRef,
		Snapshotter:   "auto",
		GCKeepMB:      defaultGCKeepMB,
		GCKeepFor:     defaultGCKeepDuration,
		HealthTimeout: defaultHealthTimeout,
		IdleStop:      defaultIdleStop,
	}
	if DefaultDigest != "" {
		cfg.Image += "@" + DefaultDigest
	}
	if v := get("APP_BUILDKIT_IMAGE"); v != "" {
		cfg.Image = v
	}
	if v := get("APP_BUILDKIT_SNAPSHOTTER"); v != "" {
		switch v {
		case "auto", "overlayfs", "fuse-overlayfs", "native":
			cfg.Snapshotter = v
		default:
			return Config{}, fmt.Errorf("buildkitd: unknown APP_BUILDKIT_SNAPSHOTTER %q", v)
		}
	}
	if v := get("APP_BUILDKIT_GC_KEEP_MB"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("buildkitd: bad APP_BUILDKIT_GC_KEEP_MB %q", v)
		}
		cfg.GCKeepMB = n
	}
	durations := []struct {
		key string
		dst *time.Duration
	}{
		{"APP_BUILDKIT_GC_KEEP_DURATION", &cfg.GCKeepFor},
		{"APP_BUILDKIT_HEALTH_TIMEOUT", &cfg.HealthTimeout},
		{"APP_BUILDKIT_IDLE_STOP", &cfg.IdleStop},
	}
	for _, d := range durations {
		v := get(d.key)
		if v == "" {
			continue
		}
		if v == "0" {
			*d.dst = 0
			continue
		}
		parsed, err := time.ParseDuration(v)
		if err != nil || parsed < 0 {
			return Config{}, fmt.Errorf("buildkitd: bad %s %q", d.key, v)
		}
		*d.dst = parsed
	}
	if rootless {
		xdg := get("XDG_RUNTIME_DIR")
		if xdg == "" {
			return Config{}, errors.New("buildkitd: rootless needs XDG_RUNTIME_DIR")
		}
		cfg.SocketDir = filepath.Join(xdg, "levelrail", "buildkit")
		cfg.ConfigDir = filepath.Join(xdg, "levelrail", "buildkit-conf")
		return cfg, nil
	}
	data := get("APP_DATA_DIR")
	if data == "" {
		return Config{}, errors.New("buildkitd: rootful needs APP_DATA_DIR")
	}
	cfg.SocketDir = filepath.Join(data, "buildkit", "run")
	cfg.ConfigDir = filepath.Join(data, "buildkit", "conf")
	return cfg, nil
}

// CheckPinned refuses an image reference without a full sha256 digest.
func CheckPinned(image string) error {
	i := strings.Index(image, sha256Prefix)
	if i < 0 || len(image)-i-len(sha256Prefix) != 64 {
		return ErrImageNotPinned
	}
	return nil
}

// tomlBody renders buildkitd.toml: the GC policy plus the snapshotter choice.
func (c Config) tomlBody() string {
	var b strings.Builder
	b.WriteString("[worker.oci]\n  gc = true\n")
	if c.Snapshotter != "auto" {
		fmt.Fprintf(&b, "  snapshotter = %q\n", c.Snapshotter)
	}
	fmt.Fprintf(&b, "  [[worker.oci.gcpolicy]]\n    all = true\n    keepBytes = %d\n    keepDuration = %d\n",
		c.GCKeepMB*1024*1024, int64(c.GCKeepFor.Seconds()))
	return b.String()
}

func ensureDir0700(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700) //nolint:gosec // directories need the execute bit
}
