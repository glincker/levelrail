// Package mox adapts the mox admin API (Sherpa JSON-RPC) to mail.Engine.
package mox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/mail"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Env knobs for the admin API client.
const (
	TimeoutEnv  = "APP_MAIL_API_TIMEOUT"
	MaxBytesEnv = "APP_MAIL_API_MAX_BYTES"
	// AdminPortEnv overrides the mox admin listener port inside the container.
	AdminPortEnv = "APP_MAIL_ADMIN_PORT"

	defaultTimeout  = 10 * time.Second
	defaultMaxBytes = 1 << 20
	// DefaultAdminPort is the hardened template's admin listener, never
	// published on the host.
	DefaultAdminPort = 8080
)

// AdminBaseURL builds the adapter address from the service's container
// address on the app network. The control plane resolves host; the admin
// port is never reachable from outside that network.
func AdminBaseURL(host string) string {
	port := strconv.FormatInt(envInt(AdminPortEnv, DefaultAdminPort), 10)
	return "http://" + net.JoinHostPort(host, port)
}

// Config configures the adapter. BaseURL is the engine's admin origin on the
// app network (see AdminBaseURL), for example http://10.0.0.5:8080. HTTPClient is for tests only.
type Config struct {
	BaseURL     string
	Credentials mail.CredentialSource
	HTTPClient  *http.Client
}

// Client talks to one mox admin API.
type Client struct {
	base     *url.URL
	creds    mail.CredentialSource
	hc       *http.Client
	maxBytes int64

	mu   sync.Mutex
	csrf string
}

var _ mail.Engine = (*Client)(nil)

// New validates the configured address and builds a client. Only http(s) is
// accepted and link-local or metadata hosts are refused up front; the dial
// guard repeats the check on the resolved address.
func New(cfg Config) (*Client, error) {
	if cfg.Credentials == nil {
		return nil, fmt.Errorf("%w: credential source required", mail.ErrInvalid)
	}
	u, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: base URL must be an absolute http or https URL", mail.ErrInvalid)
	}
	if ip, perr := netip.ParseAddr(u.Hostname()); perr == nil && !netguard.IsPrivateTargetAllowed(ip) {
		return nil, fmt.Errorf("%w: %w", mail.ErrInvalid, netguard.ErrBlockedAddress)
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = netguard.NewClientPrivateTarget()
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc.Jar = jar
	hc.Timeout = envDuration(TimeoutEnv, defaultTimeout)
	return &Client{base: u, creds: cfg.Credentials, hc: hc, maxBytes: envInt(MaxBytesEnv, defaultMaxBytes)}, nil
}

func envDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(name)); err == nil && d > 0 {
		return d
	}
	return def
}

func envInt(name string, def int64) int64 {
	if n, err := strconv.ParseInt(os.Getenv(name), 10, 64); err == nil && n > 0 {
		return n
	}
	return def
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// errNoAuth marks a session that expired so call can log in once more.
var errNoAuth = errors.New("no auth")

// call invokes fn, logging in lazily and once more if the session expired.
// secrets lists values (such as a password argument) scrubbed from errors.
func (c *Client) call(ctx context.Context, fn string, params []any, out any, secrets ...string) error {
	if params == nil {
		params = []any{}
	}
	err := c.callOnce(ctx, fn, params, out, secrets)
	if errors.Is(err, errNoAuth) {
		c.mu.Lock()
		c.csrf = ""
		c.mu.Unlock()
		err = c.callOnce(ctx, fn, params, out, secrets)
		if errors.Is(err, errNoAuth) {
			return &mail.Error{Kind: mail.ErrUnauthorized, Code: "user:noAuth"}
		}
	}
	return err
}

func (c *Client) callOnce(ctx context.Context, fn string, params []any, out any, secrets []string) error {
	csrf, err := c.session(ctx)
	if err != nil {
		return err
	}
	raw, err := c.post(ctx, fn, params, csrf, secrets)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("mail: decode %s result: %w", fn, err)
	}
	return nil
}

func (c *Client) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.csrf != "" {
		return c.csrf, nil
	}
	pw, err := c.creds.AdminPassword(ctx)
	if err != nil {
		return "", fmt.Errorf("mail: read admin credentials: %w", err)
	}
	secrets := []string{pw}
	raw, err := c.post(ctx, "LoginPrep", []any{}, "", secrets)
	if err != nil {
		return "", err
	}
	var token string
	if err := json.Unmarshal(raw, &token); err != nil || token == "" {
		return "", fmt.Errorf("mail: unexpected LoginPrep result")
	}
	raw, err = c.post(ctx, "Login", []any{token, pw}, "", secrets)
	if err != nil {
		if errors.Is(err, errNoAuth) {
			return "", &mail.Error{Kind: mail.ErrUnauthorized, Code: "user:noAuth"}
		}
		return "", err
	}
	var csrf string
	if err := json.Unmarshal(raw, &csrf); err != nil || csrf == "" {
		return "", fmt.Errorf("mail: unexpected Login result")
	}
	c.csrf = csrf
	return csrf, nil
}

func (c *Client) post(ctx context.Context, fn string, params []any, csrf string, secrets []string) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{"params": params})
	if err != nil {
		return nil, err
	}
	endpoint := c.base.JoinPath("admin", "api", fn).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("x-mox-csrf", csrf)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, scrub(fmt.Errorf("mail: %s: %w", fn, unwrapURL(err)), secrets)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		return nil, scrub(fmt.Errorf("mail: %s: read body: %w", fn, err), secrets)
	}
	if int64(len(data)) > c.maxBytes {
		return nil, fmt.Errorf("mail: %s: response exceeds %d bytes (%s)", fn, c.maxBytes, MaxBytesEnv)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mail: %s: engine returned HTTP %d", fn, resp.StatusCode)
	}
	var rr rpcResponse
	if err := json.Unmarshal(data, &rr); err != nil {
		return nil, fmt.Errorf("mail: %s: malformed response", fn)
	}
	if rr.Error != nil {
		return nil, mapError(*rr.Error, secrets)
	}
	return rr.Result, nil
}

// unwrapURL drops the *url.Error wrapper, whose text repeats the full URL.
func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func scrub(err error, secrets []string) error {
	msg := redact(err.Error(), secrets)
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

func redact(s string, secrets []string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	return s
}

// notFoundMessage matches only the final clause mox v0.0.17 really returns for a missing object
// ("no such account", "unknown domain", "alias does not exist"), so any other failure is never
// read as a missing account (a delete would then drop the record of a live mailbox).
var notFoundMessage = regexp.MustCompile(`(?:^|: )(?:(?:account|domain|alias|address|destination|message|mailbox)s? (?:does not exist|not found)|no such (?:account|domain|alias|address)|unknown domain)$`)

func mapError(e rpcError, secrets []string) error {
	msg := redact(e.Message, secrets)
	low := strings.ToLower(msg)
	var kind error
	switch {
	case e.Code == "user:noAuth":
		return errNoAuth
	case e.Code == "sherpa:badParams":
		kind = mail.ErrInvalid
	case strings.Contains(low, "already exists") || strings.Contains(low, "already in use") || strings.Contains(low, "already configured") || strings.Contains(low, "already present"):
		kind = mail.ErrExists
	case notFoundMessage.MatchString(strings.TrimSpace(low)):
		kind = mail.ErrNotFound
	case e.Code == "user:error":
		kind = mail.ErrInvalid
	default:
		return &mail.Error{Kind: errors.New("mail: engine error"), Code: e.Code, Message: msg}
	}
	return &mail.Error{Kind: kind, Code: e.Code, Message: msg}
}
