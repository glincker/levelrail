package k8sconn

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// Env switches seeding the per-request limits.
const (
	EnvTimeoutSeconds = "APP_K8S_REQUEST_TIMEOUT_SECONDS"
	EnvMaxBodyBytes   = "APP_K8S_MAX_BODY_BYTES"
)

// Config describes one cluster connection.
type Config struct {
	Creds    Credentials
	Guard    trafficpolicy.EgressGuard
	Resolver trafficpolicy.Resolver
	Timeout  time.Duration
	MaxBody  int64
}

// LimitsFromEnv returns the request timeout and body cap.
func LimitsFromEnv(getenv func(string) string) (time.Duration, int64) {
	if getenv == nil {
		getenv = os.Getenv
	}
	t, b := 15, int64(8<<20)
	if n, err := strconv.Atoi(getenv(EnvTimeoutSeconds)); err == nil && n > 0 {
		t = n
	}
	if n, err := strconv.ParseInt(getenv(EnvMaxBodyBytes), 10, 64); err == nil && n > 0 {
		b = n
	}
	return time.Duration(t) * time.Second, b
}

// Client talks to one API server through a pinned, guard-checked address.
type Client struct {
	http    *http.Client
	base    string
	token   string
	maxBody int64
}

// StatusError carries a non-2xx status without echoing the response body.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("k8sconn: api server returned %d", e.Code) }

// NewClient checks the server address once and pins every dial to it, so a
// DNS answer that changes later cannot redirect traffic to a refused host.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.Creds.Server)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, errors.New("k8sconn: server must be an https url")
	}
	res := cfg.Resolver
	if res == nil {
		res = net.DefaultResolver
	}
	target, err := cfg.Guard.CheckTarget(ctx, res, cfg.Creds.Server)
	if err != nil {
		return nil, err
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	if cfg.Creds.ServerName != "" {
		tlsCfg.ServerName = cfg.Creds.ServerName
	}
	if len(cfg.Creds.CA) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.Creds.CA) {
			return nil, errors.New("k8sconn: ca holds no certificate")
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.Creds.ClientCert != nil {
		pair, err := tls.X509KeyPair(cfg.Creds.ClientCert, cfg.Creds.ClientKey)
		if err != nil {
			return nil, errors.New("k8sconn: client certificate and key do not match")
		}
		tlsCfg.Certificates = []tls.Certificate{pair}
	}
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	pinned := target.Dial()
	tr := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: tlsCfg,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, pinned)
		},
	}
	limit := cfg.MaxBody
	if limit <= 0 {
		_, limit = LimitsFromEnv(nil)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout, _ = LimitsFromEnv(nil)
	}
	return &Client{
		http: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			// A redirect would carry the bearer token to an unchecked host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		base:    "https://" + u.Host,
		token:   cfg.Creds.Token,
		maxBody: limit,
	}, nil
}

// Do sends one request and returns the body of a 2xx response.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, classify(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, classify(err)
	}
	if int64(len(data)) > c.maxBody {
		return nil, errors.New("k8sconn: response over the size cap")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &StatusError{Code: resp.StatusCode}
	}
	return data, nil
}

// classify reduces transport errors to a class so URLs and tokens never leak.
func classify(err error) error {
	var cert x509.UnknownAuthorityError
	var host x509.HostnameError
	switch {
	case errors.As(err, &cert), errors.As(err, &host):
		return errors.New("k8sconn: tls verification failed")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("k8sconn: request timed out")
	case errors.Is(err, context.Canceled):
		return err
	}
	return errors.New("k8sconn: api server unreachable")
}
