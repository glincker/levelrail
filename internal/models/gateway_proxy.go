package models

import (
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

var errStreamIdle = errors.New("engine stream idle")

func newGatewayTransport(l GatewayLimits) *http.Transport {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: l.DialTimeout}).DialContext,
		ResponseHeaderTimeout: max(l.HeaderTimeout, 0),
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
	}
}

// idleBody cancels the upstream request when one Read blocks longer than d.
// Only time spent waiting on the engine counts, so a slow client never
// trips it and a long stream that keeps producing tokens never ends early.
type idleBody struct {
	rc     io.ReadCloser
	d      time.Duration
	timer  *time.Timer
	cancel func()
}

func newIdleBody(rc io.ReadCloser, d time.Duration, cancel func()) *idleBody {
	b := &idleBody{rc: rc, d: d, cancel: cancel}
	b.timer = time.AfterFunc(time.Hour, cancel)
	b.timer.Stop()
	return b
}

func (b *idleBody) Read(p []byte) (int, error) {
	b.timer.Reset(b.d)
	n, err := b.rc.Read(p)
	b.timer.Stop()
	return n, err
}

func (b *idleBody) Close() error {
	b.timer.Stop()
	return b.rc.Close()
}

// statusWriter records the status and byte count for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(p)
	s.bytes += int64(n)
	return n, err
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
