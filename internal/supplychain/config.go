package supplychain

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
)

// Environment variables ConfigFromEnv reads.
const (
	EnvScanEnabled   = "APP_SCAN_ENABLED"
	EnvScanner       = "APP_SCANNER"
	EnvScannerImage  = "APP_SCANNER_IMAGE"
	EnvScanTimeout   = "APP_SCAN_TIMEOUT"
	EnvPullTimeout   = "APP_SCAN_PULL_TIMEOUT"
	EnvScanMemoryMB  = "APP_SCAN_MEMORY_MB"
	EnvScanCPUs      = "APP_SCAN_CPUS"
	EnvOverrideTTL   = "APP_SCAN_OVERRIDE_TTL"
	EnvKeepPerApp    = "APP_SBOM_KEEP_PER_APP"
	EnvSweepInterval = "APP_SCAN_SWEEP_INTERVAL"
)

// Default scanner images. Pinned tags, override with APP_SCANNER_IMAGE.
const (
	DefaultTrivyImage = "docker.io/aquasec/trivy:0.65.0"
	DefaultGrypeImage = "docker.io/anchore/grype:v0.99.1"
)

// Config holds every supply chain threshold. ConfigFromEnv fills defaults.
type Config struct {
	// BuildAttest reports whether Dockerfile builds produce SBOMs at all.
	BuildAttest bool
	// Enabled is the server kill switch: false refuses every scan.
	Enabled     bool
	Scanner     string
	Image       string
	Timeout     time.Duration
	PullTimeout time.Duration
	MemoryMB    int64
	CPUs        float64
	OverrideTTL time.Duration
	// KeepPerApp is how many recent SBOM documents are kept per app.
	KeepPerApp    int
	SweepInterval time.Duration
}

// ConfigFromEnv reads Config from lookup (os.LookupEnv in production).
// Unset or malformed values fall back to the defaults.
func ConfigFromEnv(lookup func(string) (string, bool), logger *slog.Logger) Config {
	if logger == nil {
		logger = slog.Default()
	}
	r := envReader{lookup: lookup, logger: logger}
	scanner := strings.ToLower(r.str(EnvScanner, ScannerTrivy))
	image := DefaultTrivyImage
	switch scanner {
	case ScannerTrivy:
	case ScannerGrype:
		image = DefaultGrypeImage
	default:
		logger.Warn("supplychain: unknown scanner, using trivy", slog.String("env", EnvScanner), slog.String("value", scanner))
		scanner = ScannerTrivy
	}
	return Config{
		BuildAttest:   r.boolean(build.EnvAttest, false),
		Enabled:       r.boolean(EnvScanEnabled, true),
		Scanner:       scanner,
		Image:         r.str(EnvScannerImage, image),
		Timeout:       r.duration(EnvScanTimeout, 5*time.Minute),
		PullTimeout:   r.duration(EnvPullTimeout, 10*time.Minute),
		MemoryMB:      int64(r.integer(EnvScanMemoryMB, 1024)),
		CPUs:          r.float(EnvScanCPUs, 1.0),
		OverrideTTL:   r.duration(EnvOverrideTTL, time.Hour),
		KeepPerApp:    r.integer(EnvKeepPerApp, 20),
		SweepInterval: r.duration(EnvSweepInterval, time.Hour),
	}
}

type envReader struct {
	lookup func(string) (string, bool)
	logger *slog.Logger
}

func (r envReader) raw(key string) (string, bool) {
	v, ok := r.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (r envReader) bad(key, v string) {
	r.logger.Warn("supplychain: ignoring malformed setting", slog.String("env", key), slog.String("value", v))
}

func (r envReader) str(key, def string) string {
	if v, ok := r.raw(key); ok {
		return v
	}
	return def
}

func (r envReader) boolean(key string, def bool) bool {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.bad(key, v)
		return def
	}
	return b
}

func (r envReader) integer(key string, def int) int {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		r.bad(key, v)
		return def
	}
	return n
}

func (r envReader) float(key string, def float64) float64 {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		r.bad(key, v)
		return def
	}
	return f
}

func (r envReader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		r.bad(key, v)
		return def
	}
	return d
}
