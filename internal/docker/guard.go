package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	dockertypes "github.com/docker/docker/api/types"
)

const (
	envBootTimeout = "APP_DOCKER_BOOT_TIMEOUT"
	envCallTimeout = "APP_DOCKER_CALL_TIMEOUT"

	defaultBootTimeout = 10 * time.Second
	defaultCallTimeout = 30 * time.Second
	breakerHold        = 2 * time.Second
	negotiateRetryMin  = 2 * time.Second
	negotiateRetryMax  = 30 * time.Second
)

// ErrDaemonUnresponsive is returned fast while a recent call to the daemon timed out.
var ErrDaemonUnresponsive = errors.New("docker: daemon is not responding")

// streamingPathMarkers are API paths whose response headers or completion can
// legitimately take long, so they are not bounded by the per-call deadline.
var streamingPathMarkers = []string{
	"/events", "/attach", "/wait", "/exec/", "/images/create", "/build",
	"/images/load", "/images/get", "/stop", "/restart", "/prune", "/archive", "/push",
	"/commit", "/export", "/stats", "/grpc", "/session",
}

func isTrue(v string) bool { return v == "1" || v == "true" }

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// guardTransport bounds the wait for response headers on non-streaming calls
// and fails fast while the daemon has recently stopped answering, so a hung
// daemon cannot pile callers up.
type guardTransport struct {
	base      http.RoundTripper
	timeout   time.Duration
	downUntil atomic.Int64
	downSince atomic.Int64
}

func (g *guardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p := req.URL.Path
	if strings.HasSuffix(p, "/_ping") {
		return g.timed(req, min(g.timeout, defaultBootTimeout))
	}
	if strings.HasSuffix(p, "/logs") && isTrue(req.URL.Query().Get("follow")) {
		return g.base.RoundTrip(req)
	}
	for _, m := range streamingPathMarkers {
		if strings.Contains(p, m) {
			return g.base.RoundTrip(req)
		}
	}
	if time.Now().UnixNano() < g.downUntil.Load() {
		return nil, ErrDaemonUnresponsive
	}
	return g.timed(req, g.timeout)
}

func (g *guardTransport) timed(req *http.Request, d time.Duration) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	timer := time.AfterFunc(d, cancel)
	resp, err := g.base.RoundTrip(req.WithContext(ctx))
	if !timer.Stop() && req.Context().Err() == nil {
		now := time.Now()
		g.downUntil.Store(now.Add(breakerHold).UnixNano())
		g.downSince.CompareAndSwap(0, now.UnixNano())
		cancel()
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("%w: no response within %s", ErrDaemonUnresponsive, d)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	g.downUntil.Store(0)
	g.downSince.Store(0)
	resp.Body = &closeCancel{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// unresponsiveSince is when the daemon first stopped answering, zero while it answers.
func (g *guardTransport) unresponsiveSince() time.Time {
	n := g.downSince.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

type closeCancel struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *closeCancel) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// negotiator is the slice of the SDK client that API version negotiation needs.
type negotiator interface {
	Ping(ctx context.Context) (dockertypes.Ping, error)
	NegotiateAPIVersionPing(dockertypes.Ping)
}

// negotiateUpFront negotiates the API version once with a bounded wait. If
// the daemon does not answer it keeps retrying in the background until it
// does or stop is closed, instead of the SDK's lazy negotiation that holds a
// client-wide lock during the ping.
func negotiateUpFront(n negotiator, boot time.Duration, stop <-chan struct{}) {
	try := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), boot)
		defer cancel()
		ping, err := n.Ping(ctx)
		if err != nil {
			return false
		}
		n.NegotiateAPIVersionPing(ping)
		return true
	}
	if try() {
		return
	}
	slog.Warn("docker: daemon did not answer API version negotiation, retrying in background",
		slog.Duration("timeout", boot))
	go func() {
		wait := negotiateRetryMin
		for {
			select {
			case <-stop:
				return
			case <-time.After(wait):
			}
			if try() {
				slog.Info("docker: daemon is answering, API version negotiated")
				return
			}
			wait = min(wait*2, negotiateRetryMax)
		}
	}()
}
