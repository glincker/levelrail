// Package vmruntime holds host-side state for the managed VM.
package vmruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/vmruntime/lima"
)

// SchemaVersion is bumped when the file layout changes incompatibly.
const SchemaVersion = 1

// Env names. Env only seeds the file at init; the stored value wins afterwards.
const (
	EnvExperimental    = "APP_VM_EXPERIMENTAL"
	EnvMemoryMiB       = "APP_VM_MEMORY_MIB"
	EnvCPUs            = "APP_VM_CPUS"
	EnvDiskGiB         = "APP_VM_DISK_GIB"
	EnvMemoryMaxPct    = "APP_VM_MEMORY_MAX_PERCENT"
	EnvMemoryMinMiB    = "APP_VM_MEMORY_MIN_MIB"
	EnvBootTimeout     = "APP_VM_BOOT_TIMEOUT"
	EnvDashboardPort   = "APP_VM_DASHBOARD_PORT"
	EnvIngressPort     = "APP_VM_INGRESS_PORT"
	EnvUpgradeHealth   = "APP_VM_UPGRADE_HEALTH_TIMEOUT"
	EnvSocketPathMax   = "APP_VM_SOCKET_PATH_MAX"
	DefaultBootTimeout = 180 * time.Second
	// DefaultUpgradeHealth is how long vm upgrade waits for the new control plane before rolling back.
	DefaultUpgradeHealth = 5 * time.Minute
	// DefaultSocketPathMax is the unix socket path limit on macOS (sun_path, with its NUL).
	DefaultSocketPathMax = 104
)

// Guest names the host side reaches over limactl.
const (
	Instance  = "levelrail"
	TokenPath = "/var/lib/levelrail/data/desktop-token" //nolint:gosec // a guest file path, not a credential
	// ExchangePath and SessionCookie mirror internal/desktopmode.
	ExchangePath  = "/api/desktop/exchange"
	SessionCookie = "session_token"
)

// DashboardHost is the only host name desktop mode serves on, so cookies stay
// host-only and never leak across other localhost ports.
const DashboardHost = "levelrail.localhost"

// DashboardURL is the URL users and the desktop shell open for a dashboard port.
func DashboardURL(port int) string { return "http://" + DashboardHost + ":" + strconv.Itoa(port) + "/" }

// Applied is what the running instance was last started with, so status can
// show settings that wait for a restart.
type Applied struct {
	MemoryMiB     int `json:"memory_mib"`
	CPUs          int `json:"cpus"`
	DiskGiB       int `json:"disk_gib"`
	DashboardPort int `json:"dashboard_port"`
	IngressPort   int `json:"ingress_port"`
}

// Settings is the host settings file. Ports are host loopback ports.
type Settings struct {
	Version       int      `json:"version"`
	MemoryMiB     int      `json:"memory_mib"`
	CPUs          int      `json:"cpus"`
	DiskGiB       int      `json:"disk_gib"`
	DashboardPort int      `json:"dashboard_port"`
	IngressPort   int      `json:"ingress_port"`
	Applied       *Applied `json:"applied,omitempty"`
	// UpgradeHealth is a Go duration, seeded from env at init; the stored value wins.
	UpgradeHealth string         `json:"upgrade_health_timeout,omitempty"`
	UpgradeBackup *UpgradeBackup `json:"upgrade_backup,omitempty"`
}

// UpgradeBackup records the last guest data backup taken by vm upgrade.
type UpgradeBackup struct {
	Path   string `json:"path"`
	At     string `json:"at"`
	Result string `json:"result"`
}

// UpgradeTimeout is the stored health gate, else env, else the default.
func (s Settings) UpgradeTimeout(env Lookup) (time.Duration, error) {
	raw := strings.TrimSpace(s.UpgradeHealth)
	if raw == "" {
		raw, _ = env(EnvUpgradeHealth)
		raw = strings.TrimSpace(raw)
	}
	if raw == "" {
		return DefaultUpgradeHealth, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration such as 5m, got %q", EnvUpgradeHealth, raw)
	}
	return d, nil
}

// SocketPathMax reads the unix socket path limit used to choose the Lima home.
func SocketPathMax(env Lookup) (int, error) {
	n, err := IntEnv(env, EnvSocketPathMax, DefaultSocketPathMax)
	if err != nil || n == 0 {
		return DefaultSocketPathMax, err
	}
	return n, nil
}

// Runner is the part of limactl access ReadToken needs.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ReadToken returns the guest desktop token. It is held in memory only and never logged.
func ReadToken(ctx context.Context, r Runner) (string, error) {
	out, err := r.Run(ctx, "shell", Instance, "--", "sudo", "cat", TokenPath)
	if err != nil {
		return "", errors.New("read the desktop token: the VM is not running or the token is missing")
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", errors.New("the desktop token file is empty")
	}
	return tok, nil
}

// TokenExists reports whether the guest token file is present.
func TokenExists(ctx context.Context, r Runner) bool {
	_, err := r.Run(ctx, "shell", Instance, "--", "sudo", "test", "-f", TokenPath)
	return err == nil
}

// Exchange trades the token for a dashboard session on the loopback port. The Host header is
// the desktop name, which is all the control plane serves.
func Exchange(ctx context.Context, client *http.Client, port int, token string) (*http.Cookie, string, error) {
	p := strconv.Itoa(port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:"+p+ExchangePath, nil)
	if err != nil {
		return nil, "", err
	}
	req.Host = DashboardHost + ":" + p
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("exchange refused: HTTP %d", resp.StatusCode)
	}
	var body struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, "", fmt.Errorf("exchange reply: %w", err)
	}
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookie {
			return c, body.ExpiresAt, nil
		}
	}
	return nil, "", errors.New("exchange reply carried no session cookie")
}

// Lookup reads one env value, like os.LookupEnv.
type Lookup func(string) (string, bool)

// SeedFromEnv builds first-init settings from env over the ADR defaults.
// Ports stay 0 when unset: the caller picks free ones.
func SeedFromEnv(env Lookup) (Settings, error) {
	d := lima.DefaultSettings()
	s := Settings{Version: SchemaVersion, MemoryMiB: d.MemoryMiB, CPUs: d.CPUs, DiskGiB: d.DiskGiB}
	for _, f := range []struct {
		name string
		dst  *int
	}{
		{EnvMemoryMiB, &s.MemoryMiB}, {EnvCPUs, &s.CPUs}, {EnvDiskGiB, &s.DiskGiB},
		{EnvDashboardPort, &s.DashboardPort}, {EnvIngressPort, &s.IngressPort},
	} {
		v, err := IntEnv(env, f.name, *f.dst)
		if err != nil {
			return s, err
		}
		*f.dst = v
	}
	raw, _ := env(EnvUpgradeHealth)
	if raw = strings.TrimSpace(raw); raw != "" {
		if _, err := s.UpgradeTimeout(env); err != nil {
			return s, err
		}
		s.UpgradeHealth = raw
	} else {
		s.UpgradeHealth = DefaultUpgradeHealth.String()
	}
	return s, nil
}

// IntEnv returns the integer in name, or def when unset or empty.
func IntEnv(env Lookup, name string, def int) (int, error) {
	raw, ok := env(name)
	raw = strings.TrimSpace(raw)
	if !ok || raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", name, raw)
	}
	return n, nil
}

// BootTimeout reads APP_VM_BOOT_TIMEOUT as a Go duration.
func BootTimeout(env Lookup) (time.Duration, error) {
	raw, _ := env(EnvBootTimeout)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultBootTimeout, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration such as 180s, got %q", EnvBootTimeout, raw)
	}
	return d, nil
}

// Limits derives lima limits from the host and APP_VM_MEMORY_* env.
func Limits(env Lookup, hostMemMiB, hostCPUs int, homeDir, dataDir string) (lima.Limits, error) {
	pct, err := IntEnv(env, EnvMemoryMaxPct, lima.DefaultMemoryMaxPercent)
	if err != nil {
		return lima.Limits{}, err
	}
	l := lima.DefaultLimits(hostMemMiB, hostCPUs, pct, homeDir, dataDir)
	minMiB, err := IntEnv(env, EnvMemoryMinMiB, lima.DefaultMemoryMinMiB)
	if err != nil {
		return l, err
	}
	l.MemoryMinMiB = minMiB
	return l, nil
}

// ToLima converts to template settings. Sidecar is supplied by the caller.
func (s Settings) ToLima() lima.Settings {
	l := lima.DefaultSettings()
	l.MemoryMiB, l.CPUs, l.DiskGiB = s.MemoryMiB, s.CPUs, s.DiskGiB
	l.DashboardPort, l.IngressPort = s.DashboardPort, s.IngressPort
	return l
}

// Validate checks the settings through the template validator.
func (s Settings) Validate(l lima.Limits) error { return s.ToLima().Validate(l) }

// Pending lists settings whose stored value differs from the running one.
func (s Settings) Pending() []string {
	if s.Applied == nil {
		return nil
	}
	var out []string
	add := func(name string, a, b int) {
		if a != b {
			out = append(out, name)
		}
	}
	add("memory", s.MemoryMiB, s.Applied.MemoryMiB)
	add("cpus", s.CPUs, s.Applied.CPUs)
	add("disk", s.DiskGiB, s.Applied.DiskGiB)
	add("dashboard-port", s.DashboardPort, s.Applied.DashboardPort)
	add("ingress-port", s.IngressPort, s.Applied.IngressPort)
	return out
}

// DefaultPath is the settings file under the user config dir.
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Levelrail", "vm", "settings.json"), nil
}

// ErrNotFound means no settings file exists yet.
var ErrNotFound = errors.New("vm settings file not found")

// Load reads and checks the schema version of the file at path.
func Load(path string) (Settings, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is the per-user settings file
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, ErrNotFound
	}
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("vm settings %s: %w", path, err)
	}
	if s.Version != SchemaVersion {
		return Settings{}, fmt.Errorf("vm settings %s: unsupported version %d (want %d)", path, s.Version, SchemaVersion)
	}
	return s, nil
}

// Save writes s atomically with mode 0600: temp file, fsync, rename.
func Save(path string, s Settings) error {
	s.Version = SchemaVersion
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
