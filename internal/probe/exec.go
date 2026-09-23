package probe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (p *Prober) execAttempt(ctx context.Context, containerID string, cmd []string, timeout time.Duration) error {
	desc := "exec " + DescribeCommand(cmd)
	if p.exec == nil || containerID == "" {
		return &Failure{Kind: FailureExecNotReady, Reason: desc + ": exec probes are not available for this container"}
	}

	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	code, output, err := p.exec.ExecProbe(attemptCtx, containerID, cmd)
	switch {
	case errors.Is(attemptCtx.Err(), context.DeadlineExceeded):
		return &Failure{Kind: FailureTimeout, Reason: fmt.Sprintf("%s timed out after %s", desc, timeout), Err: attemptCtx.Err()}
	case err != nil:
		return &Failure{Kind: FailureExecError, Reason: fmt.Sprintf("%s could not run: %v", desc, err), Err: err}
	case code != 0:
		reason := fmt.Sprintf("%s exited %d", desc, code)
		if out := truncateOutput(output, p.limits.ExecOutputBytes); out != "" {
			reason += ": " + out
		}
		return &Failure{Kind: FailureExitCode, Reason: reason}
	}
	return nil
}

// DescribeCommand renders cmd for a status message, unwrapping the
// ["/bin/sh", "-c", script] form back to the script the operator wrote.
func DescribeCommand(cmd []string) string {
	if len(cmd) == 3 && (cmd[0] == "/bin/sh" || cmd[0] == "sh") && cmd[1] == "-c" {
		return fmt.Sprintf("%q", cmd[2])
	}
	return fmt.Sprintf("%q", strings.Join(cmd, " "))
}

// truncateOutput collapses whitespace so multi-line output fits one
// condition message, then caps it at limit bytes.
func truncateOutput(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + "...(truncated)"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// ShellCommand wraps script as the argv an exec probe runs it through.
func ShellCommand(script string) []string { return []string{"/bin/sh", "-c", script} }
