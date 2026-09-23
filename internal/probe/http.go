package probe

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// maxDrainBytes bounds how much of a response body is read so the
// connection can be reused; probe bodies are never inspected.
const maxDrainBytes = 64 * 1024

var errTooManyRedirects = errors.New("too many redirects")

func (p *Prober) httpAttempt(ctx context.Context, addr string, cfg Config, timeout time.Duration) error {
	scheme := cfg.Scheme
	if scheme == "" {
		scheme = SchemeHTTP
	}
	url := scheme + "://" + addr + cfg.Path
	desc := "GET " + url

	want, err := ParseStatusSet(cfg.ExpectedStatus)
	if err != nil {
		return &Failure{Kind: FailureStatus, Reason: desc + ": " + err.Error(), Err: err}
	}

	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
	if err != nil {
		return &Failure{Kind: FailureConnect, Reason: fmt.Sprintf("%s: build request: %v", desc, err), Err: err}
	}
	if cfg.Host != "" {
		req.Host = cfg.Host
	}

	client := p.clientFor(cfg)
	resp, err := client.Do(req)
	if err != nil {
		timedOut := errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
		return p.transportFailure(desc, cfg, timeout, timedOut, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
		_ = resp.Body.Close()
	}()

	if want.Contains(resp.StatusCode) {
		return nil
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && !cfg.followRedirects() {
		loc := resp.Header.Get("Location")
		return &Failure{
			Kind:   FailureRedirect,
			Reason: fmt.Sprintf("%s returned %d to %s; set follow_redirects or expected_status (currently %s)", desc, resp.StatusCode, loc, want),
		}
	}
	return &Failure{
		Kind:   FailureStatus,
		Reason: fmt.Sprintf("%s returned %d, expected %s", desc, resp.StatusCode, want),
	}
}

func (p *Prober) transportFailure(desc string, cfg Config, timeout time.Duration, timedOut bool, err error) error {
	var verifyErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, errTooManyRedirects):
		return &Failure{Kind: FailureTooManyHops, Reason: fmt.Sprintf("%s followed more than %d redirects; raise %s or fix the redirect loop", desc, p.limits.MaxRedirects, EnvMaxRedirects), Err: err}
	case errors.As(err, &verifyErr):
		hint := "; set tls_skip_verify for a self-signed certificate"
		if cfg.TLSSkipVerify {
			hint = ""
		}
		return &Failure{Kind: FailureTLS, Reason: fmt.Sprintf("%s: TLS verification failed: %v%s", desc, verifyErr.Err, hint), Err: err}
	case timedOut:
		return &Failure{Kind: FailureTimeout, Reason: fmt.Sprintf("%s timed out after %s", desc, timeout), Err: err}
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return &Failure{Kind: FailureConnect, Reason: fmt.Sprintf("%s: %v", desc, opErr.Err), Err: err}
	}
	return &Failure{Kind: FailureConnect, Reason: fmt.Sprintf("%s: %v", desc, err), Err: err}
}

// clientFor returns a per-attempt shallow copy of p.client with redirect
// and TLS behavior set from cfg; p.client itself is never mutated.
func (p *Prober) clientFor(cfg Config) *http.Client {
	c := *p.client
	maxHops := p.limits.MaxRedirects
	follow := cfg.followRedirects()
	c.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if !follow {
			return http.ErrUseLastResponse
		}
		if len(via) > maxHops {
			return errTooManyRedirects
		}
		return nil
	}

	if cfg.Scheme != SchemeHTTPS || (!cfg.TLSSkipVerify && cfg.Host == "") {
		return &c
	}
	base, ok := c.Transport.(*http.Transport)
	if c.Transport == nil {
		base, ok = http.DefaultTransport.(*http.Transport)
	}
	if !ok {
		return &c
	}
	t := base.Clone()
	// A throwaway transport per attempt must not pool idle connections.
	t.DisableKeepAlives = true
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if base.TLSClientConfig != nil {
		tlsCfg = base.TLSClientConfig.Clone()
	}
	if cfg.Host != "" {
		host, _, err := net.SplitHostPort(cfg.Host)
		if err != nil {
			host = cfg.Host
		}
		tlsCfg.ServerName = host
	}
	if cfg.TLSSkipVerify {
		// Opt-in per probe: in-container certificates are usually self-signed.
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // explicit tls_skip_verify on this probe
	}
	t.TLSClientConfig = tlsCfg
	c.Transport = t
	return &c
}
