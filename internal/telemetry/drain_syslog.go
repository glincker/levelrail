//go:build !windows && !plan9 && !js && !wasip1

package telemetry

import (
	"context"
	"fmt"
	"log/syslog"
	"net/url"
)

// SyslogSink writes each entry to a syslog daemon via the standard
// library's log/syslog package, which this build tag mirrors exactly:
// that package itself only builds on the platforms listed above.
// CLAUDE.md §2 already scopes this platform to Linux-only nodes, so the
// unsupported set (Windows, Plan 9, WASM) is not a real gap for a
// managed node, only for compiling this package on an unsupported dev
// machine, which the sibling stub file (drain_syslog_stub.go) covers.
type SyslogSink struct {
	writer *syslog.Writer
}

// parseSyslogTarget splits target into the network/raddr pair
// syslog.Dial takes: empty network means "local syslog daemon" (the
// same convention syslog.Dial itself uses when called with network="").
// A pure function, deliberately separated from newSyslogSink below, so
// this parsing can be unit tested without actually dialing anything.
func parseSyslogTarget(target string) (network, raddr string, err error) {
	if target == "" || target == "local" {
		return "", "", nil
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", "", fmt.Errorf("telemetry: parse syslog target %q: %w", target, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("telemetry: syslog target %q must be network://host:port", target)
	}
	return u.Scheme, u.Host, nil
}

// defaultSyslogProgramTag is used when newSyslogSink is called with "":
// a fixed, brand-independent fallback, the same reasoning
// internal/reconcile/application.defaultNetworkPrefix's own doc comment
// gives for not deriving this from brand.ShortName directly.
const defaultSyslogProgramTag = "platform"

// newSyslogSink dials target and returns a ready SyslogSink. target is
// either empty/"local" (write to this host's local syslog daemon) or
// "network://host:port" (e.g. "udp://logs.example.com:514",
// "tcp://logs.example.com:601"), the same network/address split
// syslog.Dial itself takes. programTag identifies this process in each
// syslog line (the tag syslog.New/syslog.Dial takes); callers pass the
// resolved brand.Brand.ShortName, following the same "pass the resolved
// value in, don't import internal/brand here" convention
// internal/reconcile/application.WithNetworkPrefix already establishes.
// An empty programTag falls back to defaultSyslogProgramTag.
func newSyslogSink(target, programTag string) (*SyslogSink, error) {
	network, raddr, err := parseSyslogTarget(target)
	if err != nil {
		return nil, err
	}
	if programTag == "" {
		programTag = defaultSyslogProgramTag
	}

	if network == "" {
		w, err := syslog.New(syslog.LOG_INFO, programTag)
		if err != nil {
			return nil, fmt.Errorf("telemetry: dial local syslog: %w", err)
		}
		return &SyslogSink{writer: w}, nil
	}

	w, err := syslog.Dial(network, raddr, syslog.LOG_INFO, programTag)
	if err != nil {
		return nil, fmt.Errorf("telemetry: dial syslog %q: %w", target, err)
	}
	return &SyslogSink{writer: w}, nil
}

// Send implements DrainSink. log/syslog's Writer takes no context, so
// this only checks ctx before each line rather than mid-write, enough to
// stop promptly on shutdown without needing a custom transport.
func (s *SyslogSink) Send(ctx context.Context, entries []LogEntry) error {
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		write := s.writer.Info
		if e.Stream == "stderr" {
			write = s.writer.Err
		}
		if err := write(e.Message); err != nil {
			return fmt.Errorf("telemetry: write syslog line: %w", err)
		}
	}
	return nil
}
