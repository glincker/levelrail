// Package hostdiag answers a fixed set of read-only host questions for the
// web CLI console, in process. It never executes anything and takes no input
// beyond a subcommand name chosen from a closed list.
package hostdiag

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"
)

// EnvSwitch turns host diagnostics off without turning off the console.
const EnvSwitch = "APP_CLI_CONSOLE_HOST"

// Command is the console word that routes a line here.
const Command = "host"

const subHelp = "help"

var subcommands = []string{"df", "free", "uptime", "load", "os", "service", subHelp}

// processStart is captured at package init, which runs at process start.
var processStart = time.Now()

// ErrUnsupported means the current OS has no implementation.
var ErrUnsupported = errors.New("not supported on this platform")

// UsageError is a refusal whose message is safe to show the operator.
type UsageError struct{ Reason string }

func (e *UsageError) Error() string { return e.Reason }

// Info is what the control plane itself supplies; none of it comes from the request.
type Info struct {
	DataDir    string
	Version    string
	Supervised bool
}

// Enabled reports whether host diagnostics are allowed by the environment.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvSwitch))) {
	case "off", "false", "0", "no", "disabled":
		return false
	}
	return true
}

// Subcommands returns the closed list of host subcommands.
func Subcommands() []string { return slices.Clone(subcommands) }

// Check validates a parsed line whose first word is Command and returns the subcommand.
func Check(argv []string) (string, error) {
	if len(argv) == 0 || argv[0] != Command {
		return "", &UsageError{Reason: "not a host command"}
	}
	if len(argv) == 1 {
		return subHelp, nil
	}
	sub := argv[1]
	if !slices.Contains(subcommands, sub) {
		return "", &UsageError{Reason: fmt.Sprintf("unknown host subcommand %q; available: %s", sub, strings.Join(subcommands, ", "))}
	}
	if len(argv) > 2 {
		return "", &UsageError{Reason: fmt.Sprintf("host %s takes no arguments", sub)}
	}
	return sub, nil
}

// Run produces the plain text report for a subcommand that passed Check.
func Run(sub string, info Info) (string, error) {
	if sub == subHelp {
		return helpText(), nil
	}
	if !slices.Contains(subcommands, sub) {
		return "", &UsageError{Reason: fmt.Sprintf("unknown host subcommand %q", sub)}
	}
	out, err := collect(sub, info)
	if errors.Is(err, ErrUnsupported) {
		return "", fmt.Errorf("host %s: %w", sub, ErrUnsupported)
	}
	return out, err
}

func helpText() string {
	return strings.Join([]string{
		"host diagnostics (read only, no arguments):",
		"  host df       disk usage of the data directory and /",
		"  host free     memory and swap",
		"  host uptime   time since boot",
		"  host load     load averages",
		"  host os       operating system and kernel release",
		"  host service  this control plane process",
		"  host help     this list",
	}, "\n") + "\n"
}

// Sanitize drops escape and control characters, keeping newlines and tabs.
func Sanitize(s string) string {
	s = strings.ToValidUTF8(s, "?")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
