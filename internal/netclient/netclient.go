// Package netclient builds outbound HTTP clients that always have a timeout
// and never follow redirects silently.
package netclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// DefaultTimeout is used until a live source is installed.
const DefaultTimeout = 30 * time.Second

var source atomic.Pointer[func() time.Duration]

// SetTimeoutSource installs the live net.outbound_timeout reader.
func SetTimeoutSource(f func() time.Duration) {
	if f == nil {
		source.Store(nil)
		return
	}
	source.Store(&f)
}

func timeout() time.Duration {
	if f := source.Load(); f != nil {
		if d := (*f)(); d > 0 {
			return d
		}
	}
	return DefaultTimeout
}

// ErrRedirect is returned instead of following a redirect.
var ErrRedirect = errors.New("netclient: redirect refused")

type timeoutTransport struct{ base http.RoundTripper }

func (t timeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), timeout())
	resp, err := t.base.RoundTrip(req.Clone(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

// New returns a client whose per-request timeout is read live and that refuses redirects.
func New() *http.Client {
	return &http.Client{
		Transport: timeoutTransport{base: http.DefaultTransport},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrRedirect
		},
	}
}
