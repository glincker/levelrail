// Package buildernode is the host side of a laptop builder: join config, credential
// storage, doctor checks and the lifecycle of the managed VM that runs the agent.
package buildernode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Env names. Env only seeds flag defaults; flags and the stored config win.
const (
	EnvMaxCPU        = "APP_BUILDER_MAX_CPU"
	EnvMaxMemMiB     = "APP_BUILDER_MAX_MEM_MIB"
	EnvIdleStop      = "APP_BUILDER_IDLE_STOP"
	EnvPauseBattery  = "APP_BUILDER_PAUSE_ON_BATTERY"
	EnvMinRAMMiB     = "APP_BUILDER_MIN_RAM_MIB"
	EnvMinDiskGiB    = "APP_BUILDER_MIN_DISK_GIB"
	EnvMaxClockSkew  = "APP_BUILDER_MAX_CLOCK_SKEW"
	EnvPollInterval  = "APP_BUILDER_POLL_INTERVAL"
	EnvIdleLoad      = "APP_BUILDER_IDLE_LOAD"
	EnvAgentPort     = "APP_BUILDER_AGENT_PORT"
	EnvProbeTimeout  = "APP_BUILDER_PROBE_TIMEOUT"
	DefaultMaxCPU    = 4
	DefaultMaxMemMiB = 6144
	DefaultMinRAMMiB = 8192
	DefaultMinDisk   = 40
	DefaultAgentPort = 8081
	DefaultSkew      = 30 * time.Second
	DefaultPoll      = 30 * time.Second
	DefaultProbe     = 10 * time.Second
	DefaultIdleLoad  = 0.2
	// Instance is a separate Lima instance so a builder never shares a disk with a control plane VM.
	Instance = "levelrail-builder"
	// ConfigVersion is bumped when the file layout changes incompatibly.
	ConfigVersion = 1
)

// Lookup reads one environment value.
type Lookup func(string) (string, bool)

// Config is the stored builder config. It never holds the join token.
type Config struct {
	Version        int    `json:"version"`
	Server         string `json:"server"`
	AgentAddr      string `json:"agent_addr"`
	NodeName       string `json:"node_name"`
	CAFingerprint  string `json:"ca_fingerprint,omitempty"`
	MaxCPU         int    `json:"max_cpu"`
	MaxMemMiB      int    `json:"max_mem_mib"`
	IdleStop       string `json:"idle_stop,omitempty"`
	PauseOnBattery bool   `json:"pause_on_battery"`
}

// ErrNotJoined means no config exists yet.
var ErrNotJoined = errors.New("this machine has not joined a control plane; run `levelrail builder join` first")

// ConfigPath is the config file inside dir.
func ConfigPath(dir string) string { return filepath.Join(dir, "builder.json") }

// SaveConfig writes the config with mode 0600 in a 0700 directory.
func SaveConfig(dir string, c Config) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	c.Version = ConfigVersion
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(ConfigPath(dir), append(b, '\n'))
}

// LoadConfig reads the config, or ErrNotJoined.
func LoadConfig(dir string) (Config, error) {
	var c Config
	b, err := os.ReadFile(ConfigPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return c, ErrNotJoined
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("read %s: %w", ConfigPath(dir), err)
	}
	if c.Version != ConfigVersion {
		return c, fmt.Errorf("unsupported builder config version %d", c.Version)
	}
	return c, nil
}

// writePrivate replaces path atomically with a 0600 file.
func writePrivate(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// CredStore holds the one-time join token. The file store is the portable default.
type CredStore interface {
	Save(token string) error
	Load() (string, error)
	Delete() error
}

// FileStore keeps the token in a 0600 file next to the config.
type FileStore struct{ Path string }

// NewFileStore returns the store for dir.
func NewFileStore(dir string) FileStore { return FileStore{Path: filepath.Join(dir, "builder-token")} }

// Save writes the token.
func (s FileStore) Save(token string) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	return writePrivate(s.Path, []byte(token+"\n"))
}

// Load returns the token, or an empty string when none is stored.
func (s FileStore) Load() (string, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// Delete removes the token; a missing file is fine.
func (s FileStore) Delete() error {
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// JoinInput is what `builder join` receives.
type JoinInput struct {
	Server, Token, NodeName, AgentAddr, CAFingerprint string
	Limits                                            Limits
}

// Limits are the resource and power settings of a builder.
type Limits struct {
	MaxCPU         int
	MaxMemMiB      int
	IdleStop       time.Duration
	PauseOnBattery bool
}

// LookupLimits seeds limits from env; flags override the result.
func LookupLimits(env Lookup) (Limits, error) {
	cpu, err := intEnv(env, EnvMaxCPU, DefaultMaxCPU)
	if err != nil {
		return Limits{}, err
	}
	mem, err := intEnv(env, EnvMaxMemMiB, DefaultMaxMemMiB)
	if err != nil {
		return Limits{}, err
	}
	idle, err := durEnv(env, EnvIdleStop, 0)
	if err != nil {
		return Limits{}, err
	}
	return Limits{MaxCPU: cpu, MaxMemMiB: mem, IdleStop: idle, PauseOnBattery: boolEnv(env, EnvPauseBattery)}, nil
}

// Join validates the input, stores the token and writes the config.
func Join(dir string, store CredStore, in JoinInput, env Lookup) (Config, error) {
	var c Config
	u, err := url.Parse(strings.TrimSpace(in.Server))
	if err != nil || u.Host == "" {
		return c, fmt.Errorf("--server must be a URL such as https://levelrail.example.com, got %q", in.Server)
	}
	if u.Scheme != "https" && !isLoopback(u.Hostname()) {
		return c, errors.New("--server must use https (plain http is only allowed for loopback)")
	}
	if strings.TrimSpace(in.Token) == "" {
		return c, errors.New("--token is required (create one with `levelrail nodes join-token --builder`)")
	}
	if in.Limits.MaxCPU < 1 || in.Limits.MaxMemMiB < 1 {
		return c, errors.New("--max-cpu and --max-mem must be positive")
	}
	addr := in.AgentAddr
	if addr == "" {
		port, perr := intEnv(env, EnvAgentPort, DefaultAgentPort)
		if perr != nil {
			return c, perr
		}
		addr = u.Hostname() + ":" + strconv.Itoa(port)
	}
	name := in.NodeName
	if name == "" {
		h, herr := os.Hostname()
		if herr != nil {
			return c, fmt.Errorf("--name is required, the hostname is unavailable: %w", herr)
		}
		name = h
	}
	c = Config{Server: strings.TrimRight(u.String(), "/"), AgentAddr: addr, NodeName: name, CAFingerprint: in.CAFingerprint,
		MaxCPU: in.Limits.MaxCPU, MaxMemMiB: in.Limits.MaxMemMiB, PauseOnBattery: in.Limits.PauseOnBattery}
	if in.Limits.IdleStop > 0 {
		c.IdleStop = in.Limits.IdleStop.String()
	}
	if err := store.Save(strings.TrimSpace(in.Token)); err != nil {
		return c, fmt.Errorf("store the join token: %w", err)
	}
	if err := SaveConfig(dir, c); err != nil {
		_ = store.Delete()
		return c, err
	}
	return c, nil
}

// Logout stops using this machine as a builder: it forgets the token and the config.
func Logout(dir string, store CredStore) error {
	if err := store.Delete(); err != nil {
		return err
	}
	if err := os.Remove(ConfigPath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func isLoopback(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

func intEnv(env Lookup, name string, def int) (int, error) {
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

func durEnv(env Lookup, name string, def time.Duration) (time.Duration, error) {
	raw, ok := env(name)
	raw = strings.TrimSpace(raw)
	if !ok || raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s must be a duration such as 15m, got %q", name, raw)
	}
	return d, nil
}

func boolEnv(env Lookup, name string) bool {
	v, _ := env(name)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

// Runner is the only door to limactl, so tests never boot a VM.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}
