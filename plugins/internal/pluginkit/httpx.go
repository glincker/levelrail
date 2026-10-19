package pluginkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

const (
	maxBody        = 4 << 10
	maxRetryWait   = 2 * time.Second
	defaultRetries = 2
)

// HTTP delivers one request with bounded retries and maps failures to Results.
type HTTP struct {
	Client  *http.Client
	Retries int
	Backoff time.Duration
	Sleep   func(ctx context.Context, d time.Duration) error
}

// Accept judges a 2xx response whose body may still report failure.
type Accept func(body []byte) (ok bool, detail string)

func (h HTTP) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (h HTTP) sleep(ctx context.Context, d time.Duration) error {
	if h.Sleep != nil {
		return h.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Deliver sends the request built by build, retrying network errors, 5xx and
// short 429s. Every detail it returns has secrets redacted.
func (h HTTP) Deliver(ctx context.Context, build func() (*http.Request, error), accept Accept, secrets ...string) Result {
	retries := h.Retries
	if retries == 0 {
		retries = defaultRetries
	}
	backoff := h.Backoff
	if backoff == 0 {
		backoff = 400 * time.Millisecond
	}
	var last Result
	for attempt := 0; attempt <= retries; attempt++ {
		req, err := build()
		if err != nil {
			return Result{Detail: Redact("build request: "+err.Error(), secrets...)}
		}
		resp, err := h.client().Do(req.WithContext(ctx))
		if err != nil {
			last = Result{Retryable: true, Detail: Redact("request failed: "+err.Error(), secrets...)}
			if ctx.Err() != nil || attempt == retries || h.sleep(ctx, backoff<<attempt) != nil {
				return last
			}
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		_ = resp.Body.Close()
		code := resp.StatusCode
		switch {
		case code >= 200 && code < 300:
			if accept != nil {
				if ok, detail := accept(body); !ok {
					return Result{Detail: Redact(detail, secrets...)}
				}
			}
			return Result{Delivered: true, Detail: "delivered"}
		case code == http.StatusUnauthorized || code == http.StatusForbidden:
			return Result{Detail: fmt.Sprintf("authentication failed (HTTP %d): check the credential", code)}
		case code == http.StatusTooManyRequests:
			wait := RetryAfter(resp.Header.Get("Retry-After"), body)
			last = Result{Retryable: true, Detail: fmt.Sprintf("rate limited (HTTP 429), retry after %s", wait.Round(time.Second))}
			if attempt == retries || wait > maxRetryWait || h.sleep(ctx, wait) != nil {
				return last
			}
		case code >= 500:
			last = Result{Retryable: true, Detail: Redact(fmt.Sprintf("service error (HTTP %d): %s", code, Snippet(body)), secrets...)}
			if attempt == retries || h.sleep(ctx, backoff<<attempt) != nil {
				return last
			}
		default:
			return Result{Detail: Redact(fmt.Sprintf("rejected (HTTP %d): %s", code, Snippet(body)), secrets...)}
		}
	}
	return last
}

// RetryAfter reads a Retry-After header in seconds, or a JSON retry_after_ms.
func RetryAfter(header string, body []byte) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	var b struct {
		Ms int `json:"retry_after_ms"`
	}
	if json.Unmarshal(body, &b) == nil && b.Ms > 0 {
		return time.Duration(b.Ms) * time.Millisecond
	}
	return time.Second
}

// Snippet is a short, single-line excerpt of a response body.
func Snippet(body []byte) string {
	return Truncate(strings.Join(strings.Fields(string(body)), " "), 200)
}

// Redact replaces every non-empty secret in s.
func Redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	return s
}

// Truncate cuts s to at most n runes, marking the cut.
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "..."
}

// BaseURL validates an operator-supplied service address: https only unless
// allowHTTP, a host, and no embedded credentials.
func BaseURL(raw string, allowHTTP bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, errors.New("invalid config: base URL is missing or malformed")
	}
	if u.Scheme != "https" && (!allowHTTP || u.Scheme != "http") {
		return nil, errors.New("invalid config: base URL must use https")
	}
	if u.User != nil {
		return nil, errors.New("invalid config: base URL must not contain credentials")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

// Text renders an event as a short plain-text message.
func Text(ev *pluginpb.Event) string {
	var b strings.Builder
	if st := strings.TrimSpace(ev.GetStatus()); st != "" {
		b.WriteString("[" + strings.ToUpper(st) + "] ")
	}
	b.WriteString(strings.TrimSpace(ev.GetTitle()))
	if body := strings.TrimSpace(ev.GetBody()); body != "" {
		b.WriteString("\n" + body)
	}
	if sev := strings.TrimSpace(ev.GetSeverity()); sev != "" {
		b.WriteString("\nSeverity: " + sev)
	}
	if l := strings.TrimSpace(ev.GetLink()); l != "" {
		b.WriteString("\n" + l)
	}
	return b.String()
}
