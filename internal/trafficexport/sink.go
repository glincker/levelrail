package trafficexport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Sink delivers a batch of events to one destination.
type Sink interface {
	Send(ctx context.Context, events []Event) error
	Name() string
}

// StatusError is a non-2xx vendor response.
type StatusError struct {
	Sink string
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: unexpected status %d", e.Sink, e.Code)
}

// Retryable reports whether trying again could succeed.
func (e *StatusError) Retryable() bool {
	return e.Code >= 500 || e.Code == http.StatusRequestTimeout || e.Code == http.StatusTooManyRequests
}

var errBlockedTarget = errors.New("target address is loopback or link-local; set " + EnvAllowPrivate + "=1 to allow")

func blockedAddr(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsUnspecified()
}

// newClient builds a client that checks the resolved address at connect time,
// so DNS tricks and redirects cannot reach a blocked target. No proxy is used
// because a proxy would connect on our behalf and skip the check.
func newClient(o HTTPOptions) *http.Client {
	dialer := &net.Dialer{Timeout: o.Timeout}
	if !o.AllowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return fmt.Errorf("parse dial address: %w", err)
			}
			if blockedAddr(ap.Addr()) {
				return errBlockedTarget
			}
			return nil
		}
	}
	return &http.Client{
		Timeout: o.Timeout,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: o.Timeout,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func checkTarget(raw string, o HTTPOptions) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("url must be http or https with a host")
	}
	if !o.AllowPrivate {
		if a, err := netip.ParseAddr(u.Hostname()); err == nil && blockedAddr(a) {
			return errBlockedTarget
		}
	}
	return nil
}

// postBody sends body and maps failures to errors that never contain the URL
// (which may hold a token) or response bodies.
func postBody(ctx context.Context, c *http.Client, sink, target, contentType string, headers map[string]string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: build request: %w", sink, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%s: post: %w", sink, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &StatusError{Sink: sink, Code: resp.StatusCode}
	}
	return nil
}
