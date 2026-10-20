package dataexport

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// syslogSender writes RFC 5424 messages over TCP or TLS with RFC 6587
// octet-counting framing, reconnecting on the next batch after any error.
type syslogSender struct {
	scheme   string
	hostport string
	tag      string
	hostname string

	mu   sync.Mutex
	conn net.Conn
}

func newSyslogSender(endpoint, tag string) (*syslogSender, error) {
	scheme, hostport, err := ParseSyslogEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	return &syslogSender{scheme: scheme, hostport: hostport, tag: syslogToken(tag, 48, "app"), hostname: syslogToken(host, 255, "-")}, nil
}

// syslogToken makes s a valid RFC 5424 header field: printable ASCII without
// spaces, truncated to max, or fallback when nothing is left.
func syslogToken(s string, limit int, fallback string) string {
	var b strings.Builder
	for _, r := range s {
		if r > 32 && r < 127 {
			b.WriteRune(r)
		}
		if b.Len() >= limit {
			break
		}
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}

func syslogSeverity(level string) int {
	switch level {
	case "error":
		return 3
	case "warn":
		return 4
	case "debug":
		return 7
	}
	return 6
}

// formatSyslog renders one record as an RFC 5424 message (facility local0).
func (s *syslogSender) formatSyslog(r LogRecord) string {
	pri := 16*8 + syslogSeverity(r.Level)
	msg := strings.ReplaceAll(r.Message, "\x00", "")
	return fmt.Sprintf("<%d>1 %s %s %s - %s - %s",
		pri, r.Time.UTC().Format("2006-01-02T15:04:05.000000Z07:00"), s.hostname,
		syslogToken(s.tag+"."+r.App, 48, s.tag), syslogToken(r.Stream, 32, "-"), msg)
}

func (s *syslogSender) dial(ctx context.Context) (net.Conn, error) {
	d := guardedDialer()
	raw, err := d.DialContext(ctx, "tcp", s.hostport)
	if err != nil {
		return nil, fmt.Errorf("dial syslog %s: %w", s.hostport, err)
	}
	if s.scheme != "tls" {
		return raw, nil
	}
	host, _, _ := net.SplitHostPort(s.hostport)
	tc := tls.Client(raw, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("syslog tls handshake with %s: %w", s.hostport, err)
	}
	return tc, nil
}

func (s *syslogSender) send(ctx context.Context, recs []LogRecord) error {
	var buf bytes.Buffer
	for _, r := range recs {
		m := s.formatSyslog(r)
		buf.WriteString(strconv.Itoa(len(m)))
		buf.WriteByte(' ')
		buf.WriteString(m)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		c, err := s.dial(ctx)
		if err != nil {
			return err
		}
		s.conn = c
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = s.conn.SetWriteDeadline(dl)
	} else {
		_ = s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	}
	if _, err := s.conn.Write(buf.Bytes()); err != nil {
		_ = s.conn.Close()
		s.conn = nil
		return fmt.Errorf("write syslog batch: %w", err)
	}
	return nil
}

func (s *syslogSender) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}
