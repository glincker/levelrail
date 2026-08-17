//go:build windows || plan9 || js || wasip1

package telemetry

import (
	"context"
	"fmt"
)

// newSyslogSink is a stub on platforms log/syslog itself doesn't
// support (see drain_syslog.go's own build tag and doc comment): this
// platform is Linux-only in production (CLAUDE.md §2), so a syslog
// drain is simply unavailable when this package happens to be built for
// one of these targets, e.g. a contributor's own dev machine.
func newSyslogSink(_ string) (*SyslogSink, error) {
	return nil, fmt.Errorf("telemetry: syslog log drains are not supported on this platform")
}

// SyslogSink has no fields on this build: every method is unreachable
// since newSyslogSink above always errors.
type SyslogSink struct{}

// Send is unreachable; see SyslogSink's own doc comment.
func (s *SyslogSink) Send(_ context.Context, _ []LogEntry) error {
	return fmt.Errorf("telemetry: syslog log drains are not supported on this platform")
}
