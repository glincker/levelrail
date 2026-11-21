// Package macosrt holds the macOS runtime pieces that run on a Mac host: launchd units and doctor checks.
package macosrt

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Role names which Levelrail process a launchd unit runs.
type Role string

const (
	// RoleControlPlane runs the control plane binary.
	RoleControlPlane Role = "control-plane"
	// RoleAgent runs the node agent.
	RoleAgent Role = "agent"
)

const (
	labelPrefix = "com.levelrail."
	// DefaultThrottleSeconds is the launchd respawn gap, so a missing identity does not spin.
	DefaultThrottleSeconds = 30
)

// Label is the launchd job label for a role.
func Label(r Role) string { return labelPrefix + string(r) }

// ParseRole accepts the CLI spelling of a role.
func ParseRole(s string) (Role, error) {
	switch Role(s) {
	case RoleControlPlane, RoleAgent:
		return Role(s), nil
	}
	return "", fmt.Errorf("unknown role %q (control-plane or agent)", s)
}

// Spec describes one launchd unit.
type Spec struct {
	Role            Role
	Binary          string
	Args            []string
	Env             map[string]string
	LogDir          string
	WorkDir         string
	ThrottleSeconds int
}

// secretWords keep credentials out of a plist, which any local user can read.
var secretWords = []string{"TOKEN", "SECRET", "PASSWORD", "KEY", "CREDENTIAL"}

// Validate rejects specs that would write a bad or leaky unit.
func (s Spec) Validate() error {
	if _, err := ParseRole(string(s.Role)); err != nil {
		return err
	}
	if !filepath.IsAbs(s.Binary) {
		return fmt.Errorf("binary %q must be an absolute path", s.Binary)
	}
	if !filepath.IsAbs(s.LogDir) {
		return fmt.Errorf("log dir %q must be an absolute path", s.LogDir)
	}
	if s.WorkDir != "" && !filepath.IsAbs(s.WorkDir) {
		return fmt.Errorf("work dir %q must be an absolute path", s.WorkDir)
	}
	if s.ThrottleSeconds < 0 {
		return errors.New("throttle seconds must not be negative")
	}
	for k := range s.Env {
		if k == "" || strings.ContainsAny(k, "= \t\r\n") {
			return fmt.Errorf("invalid env name %q", k)
		}
		up := strings.ToUpper(k)
		for _, w := range secretWords {
			if strings.Contains(up, w) {
				return fmt.Errorf("env %s looks like a secret; keep it in the service env file, not the unit", k)
			}
		}
	}
	return checkArgs(s.Args)
}

// checkArgs refuses launch args that carry a credential, since the unit is
// readable by other local users: secret-named flags with a value, and URLs
// with userinfo.
func checkArgs(args []string) error {
	for i, a := range args {
		if u, err := url.Parse(a); err == nil && u.Scheme != "" && u.Host != "" && u.User != nil {
			return errors.New("an argument is a URL with credentials; keep it in the service env file, not the unit")
		}
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		up := strings.ToUpper(name)
		for _, w := range secretWords {
			if !strings.Contains(up, w) {
				continue
			}
			nextIsValue := i+1 < len(args) && !strings.HasPrefix(args[i+1], "-")
			if (hasVal && val != "") || (!hasVal && nextIsValue) {
				return fmt.Errorf("argument --%s looks like a secret; keep it in the service env file, not the unit", name)
			}
		}
	}
	return nil
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Plist renders the unit. Output is deterministic so reinstall is a no-op diff.
func (s Spec) Plist() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	throttle := s.ThrottleSeconds
	if throttle == 0 {
		throttle = DefaultThrottleSeconds
	}
	var b bytes.Buffer
	w := func(f string, a ...any) { _, _ = fmt.Fprintf(&b, f, a...) }
	w("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	w("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	w("<plist version=\"1.0\">\n<dict>\n")
	w("\t<key>Label</key>\n\t<string>%s</string>\n", esc(Label(s.Role)))
	w("\t<key>ProgramArguments</key>\n\t<array>\n\t\t<string>%s</string>\n", esc(s.Binary))
	for _, a := range s.Args {
		w("\t\t<string>%s</string>\n", esc(a))
	}
	w("\t</array>\n")
	if len(s.Env) > 0 {
		keys := make([]string, 0, len(s.Env))
		for k := range s.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		for _, k := range keys {
			w("\t\t<key>%s</key>\n\t\t<string>%s</string>\n", esc(k), esc(s.Env[k]))
		}
		w("\t</dict>\n")
	}
	if s.WorkDir != "" {
		w("\t<key>WorkingDirectory</key>\n\t<string>%s</string>\n", esc(s.WorkDir))
	}
	w("\t<key>RunAtLoad</key>\n\t<true/>\n")
	w("\t<key>KeepAlive</key>\n\t<true/>\n")
	w("\t<key>ThrottleInterval</key>\n\t<integer>%d</integer>\n", throttle)
	log := filepath.Join(s.LogDir, "levelrail-"+string(s.Role)+".log")
	w("\t<key>StandardOutPath</key>\n\t<string>%s</string>\n", esc(log))
	w("\t<key>StandardErrorPath</key>\n\t<string>%s</string>\n", esc(log))
	w("</dict>\n</plist>\n")
	return b.Bytes(), nil
}

// Runner runs launchctl. Tests pass a fake; production never runs it off macOS.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// Manager installs units into the per-user LaunchAgents directory.
type Manager struct {
	Dir    string // usually ~/Library/LaunchAgents
	UID    int
	Runner Runner
}

func (m Manager) path(r Role) string { return filepath.Join(m.Dir, Label(r)+".plist") }
func (m Manager) domain() string     { return "gui/" + strconv.Itoa(m.UID) }
func (m Manager) target(r Role) string {
	return m.domain() + "/" + Label(r)
}

// Install writes the plist atomically and (re)loads it. It returns the plist path.
func (m Manager) Install(ctx context.Context, s Spec) (string, error) {
	raw, err := s.Plist()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil { //nolint:gosec // LaunchAgents is world-readable by design
		return "", err
	}
	if err := os.MkdirAll(s.LogDir, 0o700); err != nil {
		return "", err
	}
	p := m.path(s.Role)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	// A not-loaded job makes bootout fail; that is the normal first-install case.
	_, _ = m.Runner.Run(ctx, "bootout", m.target(s.Role))
	if _, err := m.Runner.Run(ctx, "bootstrap", m.domain(), p); err != nil {
		return p, fmt.Errorf("launchctl bootstrap: %w", err)
	}
	return p, nil
}

// Uninstall unloads and removes the unit. Missing pieces are not errors.
func (m Manager) Uninstall(ctx context.Context, r Role) (bool, error) {
	_, _ = m.Runner.Run(ctx, "bootout", m.target(r))
	err := os.Remove(m.path(r))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// State is what Status reports for one role.
type State struct {
	Role      Role   `json:"role"`
	Label     string `json:"label"`
	Path      string `json:"path"`
	Installed bool   `json:"installed"`
	Loaded    bool   `json:"loaded"`
}

// Status reports whether the plist exists and launchd has the job loaded.
func (m Manager) Status(ctx context.Context, r Role) State {
	st := State{Role: r, Label: Label(r), Path: m.path(r)}
	if _, err := os.Stat(st.Path); err == nil {
		st.Installed = true
	}
	if _, err := m.Runner.Run(ctx, "print", m.target(r)); err == nil {
		st.Loaded = true
	}
	return st
}
