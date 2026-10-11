package dockerguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SettingsFile is the operator's persisted mode choice under the data dir.
const SettingsFile = "docker-guard.json"

// Mode sources, reported so an operator knows where to change it.
const (
	SourceEnv      = "env"
	SourceSettings = "settings"
	SourceDefault  = "default"
)

// ErrPinnedByEnv means APP_DOCKER_GUARD is set, so the API cannot change it.
var ErrPinnedByEnv = errors.New("dockerguard: mode is pinned by " + EnvMode)

// Settings is the persisted part of the guard configuration.
type Settings struct {
	Mode Mode `json:"mode,omitempty"`
	// AuditSince is when audit mode was first observed, so the attention
	// feed can tell a week of clean observation from a fresh install.
	AuditSince time.Time `json:"audit_since,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	UpdatedBy  string    `json:"updated_by,omitempty"`
}

// LoadSettings reads path; a missing file is the zero Settings.
func LoadSettings(path string) (Settings, error) {
	var s Settings
	b, err := os.ReadFile(path) //nolint:gosec // path is derived from the operator's data dir
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("dockerguard: read settings: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, fmt.Errorf("dockerguard: parse settings %s: %w", path, err)
	}
	return s, nil
}

// SaveSettings writes path atomically with mode 0600.
func SaveSettings(path string, s Settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("dockerguard: encode settings: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".docker-guard-*.json")
	if err != nil {
		return fmt.Errorf("dockerguard: write settings: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("dockerguard: write settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("dockerguard: write settings: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("dockerguard: write settings: %w", err)
	}
	return nil
}

// Resolve picks the effective mode: the env var wins, then the settings
// file, then DefaultMode. An invalid value anywhere resolves to enforce.
func Resolve(lookup func(string) (string, bool), s Settings) (Mode, string, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if v, ok := lookup(EnvMode); ok && strings.TrimSpace(v) != "" {
		m, err := ParseMode(v)
		return m, SourceEnv, err
	}
	if s.Mode != "" {
		m, err := ParseMode(string(s.Mode))
		return m, SourceSettings, err
	}
	return DefaultMode, SourceDefault, nil
}

// Status is what the API and doctor report.
type Status struct {
	// Mode is the configured mode, Running whether the socket is up, and
	// Effective what the running guard does now.
	Mode            Mode        `json:"mode"`
	Source          string      `json:"source"`
	Effective       Mode        `json:"effective"`
	Running         bool        `json:"running"`
	Socket          string      `json:"socket,omitempty"`
	Upstream        string      `json:"upstream,omitempty"`
	RestartRequired bool        `json:"restart_required"`
	ConfigError     string      `json:"config_error,omitempty"`
	StartError      string      `json:"start_error,omitempty"`
	AuditSince      time.Time   `json:"audit_since,omitempty"`
	SinceBoot       []RuleCount `json:"since_boot"`
	Rules           []string    `json:"rules"`
}

// Controller owns the guard's configuration for the API: read status,
// switch mode, persist the choice.
type Controller struct {
	mu        sync.Mutex
	path      string
	lookup    func(string) (string, bool)
	server    *Server
	upstream  string
	startErr  error
	bootMode  Mode
	settings  Settings
	configErr error
}

// NewController wraps server (nil when the guard is off or failed to start).
func NewController(settingsPath string, lookup func(string) (string, bool), server *Server, upstream string, startErr error) *Controller {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	c := &Controller{path: settingsPath, lookup: lookup, server: server, upstream: upstream, startErr: startErr}
	c.settings, c.configErr = LoadSettings(settingsPath)
	mode, _, err := Resolve(lookup, c.settings)
	if err != nil && c.configErr == nil {
		c.configErr = err
	}
	c.bootMode = mode
	return c
}

// NoteAuditStart stamps AuditSince the first time audit mode runs.
func (c *Controller) NoteAuditStart(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bootMode != ModeAudit || !c.settings.AuditSince.IsZero() {
		return nil
	}
	c.settings.AuditSince = now.UTC()
	return SaveSettings(c.path, c.settings)
}

// Status reports the current configuration and counters.
func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	mode, source, err := Resolve(c.lookup, c.settings)
	st := Status{Mode: mode, Source: source, Upstream: c.upstream, AuditSince: c.settings.AuditSince, Rules: AllRules, SinceBoot: []RuleCount{}, Effective: ModeOff}
	if err != nil {
		st.ConfigError = err.Error()
	} else if c.configErr != nil {
		st.ConfigError = c.configErr.Error()
	}
	if c.startErr != nil {
		st.StartError = c.startErr.Error()
	}
	if c.server != nil {
		st.Running, st.Socket = true, c.server.Socket
		st.Effective = c.server.Guard.Mode()
		st.SinceBoot = c.server.Guard.Stats().Snapshot()
	}
	st.RestartRequired = (mode == ModeOff) == st.Running
	return st
}

// SetMode persists mode and applies it live when the guard is running.
func (c *Controller) SetMode(mode Mode, actor string, now time.Time) (Status, error) {
	if _, err := ParseMode(string(mode)); err != nil || mode == "" {
		return Status{}, fmt.Errorf("dockerguard: mode %q is not one of off, audit, enforce", mode)
	}
	if v, ok := c.lookup(EnvMode); ok && strings.TrimSpace(v) != "" {
		return Status{}, ErrPinnedByEnv
	}
	c.mu.Lock()
	next := c.settings
	next.Mode, next.UpdatedAt, next.UpdatedBy = mode, now.UTC(), actor
	if mode == ModeAudit && next.AuditSince.IsZero() {
		next.AuditSince = now.UTC()
	}
	if err := SaveSettings(c.path, next); err != nil {
		c.mu.Unlock()
		return Status{}, err
	}
	c.settings = next
	if c.server != nil {
		c.server.Guard.SetMode(mode)
	}
	c.mu.Unlock()
	return c.Status(), nil
}
