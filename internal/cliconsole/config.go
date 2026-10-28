package cliconsole

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Environment knobs. APP_CLI_CONSOLE=off wins over the Settings switch.
const (
	EnvKillSwitch     = "APP_CLI_CONSOLE"
	EnvIdleTimeout    = "APP_CLI_CONSOLE_IDLE_TIMEOUT"
	EnvCommandTimeout = "APP_CLI_CONSOLE_COMMAND_TIMEOUT"
	EnvMaxOutput      = "APP_CLI_CONSOLE_MAX_OUTPUT_BYTES"
	EnvMaxLine        = "APP_CLI_CONSOLE_MAX_LINE_BYTES"
	EnvMaxSession     = "APP_CLI_CONSOLE_MAX_SESSION"
	EnvOpensPerMinute = "APP_CLI_CONSOLE_OPENS_PER_MINUTE"
	EnvBinary         = "APP_CLI_BINARY"
	EnvRunAsUser      = "APP_CLI_CONSOLE_USER"
)

// Config is the console's resolved environment.
type Config struct {
	IdleTimeout    time.Duration
	CommandTimeout time.Duration
	MaxSession     time.Duration
	MaxOutputBytes int64
	MaxLineBytes   int
	OpensPerMinute int
	RunAsUser      string
}

// KilledByEnv reports whether the environment switches the console off.
func KilledByEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvKillSwitch))) {
	case "off", "false", "0", "no", "disabled":
		return true
	}
	return false
}

// LoadConfig reads the environment, falling back to safe defaults on bad values.
func LoadConfig() Config {
	return Config{
		IdleTimeout:    envDuration(EnvIdleTimeout, 10*time.Minute),
		CommandTimeout: envDuration(EnvCommandTimeout, 2*time.Minute),
		MaxSession:     envDuration(EnvMaxSession, time.Hour),
		MaxOutputBytes: int64(envInt(EnvMaxOutput, 1<<20)),
		MaxLineBytes:   envInt(EnvMaxLine, 4096),
		OpensPerMinute: envInt(EnvOpensPerMinute, 6),
		RunAsUser:      envString(EnvRunAsUser, "nobody"),
	}
}

func envDuration(name string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envInt(name string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envString(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

// ErrBinaryMissing means no usable CLI binary was found.
var ErrBinaryMissing = errors.New("CLI binary not found")

// LocateBinary finds the CLI: APP_CLI_BINARY, else "<binaryName>-cli" next
// to the running control plane. Never PATH, so the server's PATH cannot pick it.
func LocateBinary(binaryName string) (string, error) {
	if p := strings.TrimSpace(os.Getenv(EnvBinary)); p != "" {
		return vetBinary(p)
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("%w: resolve own executable: %w", ErrBinaryMissing, err)
	}
	if resolved, rerr := filepath.EvalSymlinks(self); rerr == nil {
		self = resolved
	}
	return vetBinary(filepath.Join(filepath.Dir(self), binaryName+"-cli"))
}

// vetBinary refuses a relative path, a non regular file, a non executable or
// a group or world writable file, so the console never runs a swappable binary.
func vetBinary(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: %s must be an absolute path", ErrBinaryMissing, EnvBinary)
	}
	fi, err := os.Stat(p) //nolint:gosec // operator-set env or a path next to our own executable, never request input
	if err != nil {
		return "", fmt.Errorf("%w at %s: set %s or install it next to the control plane", ErrBinaryMissing, p, EnvBinary)
	}
	mode := fi.Mode()
	if !mode.IsRegular() || mode.Perm()&0o111 == 0 {
		return "", fmt.Errorf("%w: %s is not an executable file", ErrBinaryMissing, p)
	}
	if mode.Perm()&0o022 != 0 {
		return "", fmt.Errorf("%w: %s is group or world writable", ErrBinaryMissing, p)
	}
	return p, nil
}
