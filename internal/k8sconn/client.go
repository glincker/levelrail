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
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/trafficpolicy"
)

// Fallbacks for callers that build a Config without live limits.
const (
	defaultMaxBody = 8 << 20
	defaultTimeout = 15 * time.Second
)

// Config describes one cluster connection.
type Config struct {
	Creds    Credentials
	Guard    trafficpolicy.EgressGuard
	Resolver trafficpolicy.Resolver
	Timeout  time.Duration
	MaxBody  int64
}

// Limits are the live runtime settings that bound one cluster call.
type Limits struct {
	Timeout       time.Duration
	MaxBody       int64
	KubeconfigMax int
}

// SettingsReader is the slice of the runtime settings service k8sconn needs.
type SettingsReader interface {
	Limit(ctx context.Context, key string) int
	Duration(ctx context.Context, key string) time.Duration
	Bool(ctx context.Context, key string) bool
}

// LoadLimits reads the limits live, so a settings change applies to the next call.
func LoadLimits(ctx context.Context, s SettingsReader) Limits {
	return Limits{
		Timeout:       s.Duration(ctx, runtimesettings.KeyK8sRequestTimeout),
		MaxBody:       int64(s.Limit(ctx, runtimesettings.KeyK8sMaxBodyBytes)),
		KubeconfigMax: s.Limit(ctx, runtimesettings.KeyK8sKubeconfigMaxBytes),
	}
}

// AllowPrivateAPI reports whether private and loopback API servers may be dialed.
func AllowPrivateAPI(ctx context.Context, s SettingsReader) bool {
	return s.Bool(ctx, runtimesettings.KeyK8sAllowPrivateAPI)
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

// ParseServer accepts a plain https API server URL. Credentials, query and
// fragment are refused so a secret can never be stored, audited or returned.
func ParseServer(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, errors.New("k8sconn: server must be an https url")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, errors.New("k8sconn: server url must not carry credentials, a query or a fragment")
	}
	return u, nil
}

// NewClient checks the server address once and pins every dial to it, so a
// DNS answer that changes later cannot redirect traffic to a refused host.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	u, err := ParseServer(cfg.Creds.Server)
	if err != nil {
		return nil, err
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
		// One transport per client is never reused, so idle sockets would only leak.
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, pinned)
		},
	}
	limit, timeout := cfg.MaxBody, cfg.Timeout
	if limit <= 0 {
		limit = defaultMaxBody
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		http: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			// A redirect would carry the bearer token to an unchecked host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		base:    "https://" + u.Host + strings.TrimRight(u.EscapedPath(), "/"),
		token:   cfg.Creds.Token,
		maxBody: limit,
	}, nil
}

// Do sends one request and returns the body of a 2xx response.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	return c.doType(ctx, method, path, "application/json", body)
}

func (c *Client) doType(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
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
		req.Header.Set("Content-Type", contentType)
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
