package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Failure classes stored in Result.Reason.
const (
	ReasonTimeout       = "timeout"
	ReasonBlocked       = "blocked_address"
	ReasonConnect       = "connect_failed"
	ReasonStatus        = "status_mismatch"
	ReasonKeyword       = "keyword_missing"
	ReasonRedirects     = "redirect_limit"
	ReasonBadRequest    = "bad_request"
	ReasonSecretMissing = "secret_unavailable"
)

var errTooManyRedirects = errors.New("monitor: too many redirects")

// Checker runs one probe. Every dial goes through the netguard address check.
type Checker struct {
	cfg      Config
	follow   *http.Client
	noFollow *http.Client
	dialer   *net.Dialer
	now      func() time.Time
}

// NewChecker builds a Checker whose connections refuse internal addresses
// unless the netguard private opt-in env is set.
func NewChecker(cfg Config) *Checker {
	base := netguard.NewClient()
	return &Checker{
		cfg: cfg,
		follow: &http.Client{Transport: base.Transport, CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > cfg.MaxRedirects {
				return errTooManyRedirects
			}
			return nil
		}},
		noFollow: &http.Client{Transport: base.Transport, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		dialer: &net.Dialer{Control: guardControl},
		now:    time.Now,
	}
}

func guardControl(_, address string, _ syscall.RawConn) error {
	if netguard.AllowPrivate() {
		return nil
	}
	ap, err := netip.ParseAddrPort(address)
	if err != nil || netguard.IsBlocked(ap.Addr()) {
		return netguard.ErrBlockedAddress
	}
	return nil
}

// Check probes m once. headers holds resolved header values by canonical name.
func (c *Checker) Check(ctx context.Context, m Monitor, headers map[string]string) Result {
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()
	start := c.now()
	res := Result{MonitorID: m.ID, At: start.Unix()}
	if m.Kind == KindTCP {
		c.checkTCP(ctx, m, &res)
	} else {
		c.checkHTTP(ctx, m, headers, &res)
	}
	res.LatencyMS = int(c.now().Sub(start) / time.Millisecond)
	return res
}

func (c *Checker) checkTCP(ctx context.Context, m Monitor, res *Result) {
	conn, err := c.dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.Spec.Host, strconv.Itoa(m.Spec.Port)))
	if err != nil {
		res.Reason = classify(err)
		return
	}
	_ = conn.Close()
	res.OK = true
}

func (c *Checker) checkHTTP(ctx context.Context, m Monitor, headers map[string]string, res *Result) {
	req, err := http.NewRequestWithContext(ctx, m.Spec.Method, m.Spec.URL, nil)
	if err != nil {
		res.Reason = ReasonBadRequest
		return
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := c.noFollow
	if m.Spec.FollowRedirects {
		client = c.follow
	}
	resp, err := client.Do(req)
	if err != nil {
		res.Reason = classify(err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	res.Code = resp.StatusCode
	if !statusExpected(m.Spec.Expect, resp.StatusCode) {
		res.Reason = ReasonStatus
		return
	}
	if m.Spec.Contains != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.cfg.BodyMaxBytes)))
		if err != nil {
			res.Reason = classify(err)
			return
		}
		if !strings.Contains(string(body), m.Spec.Contains) {
			res.Reason = ReasonKeyword
			return
		}
	}
	res.OK = true
}

func statusExpected(ranges []StatusRange, code int) bool {
	for _, r := range ranges {
		if code >= r.Min && code <= r.Max {
			return true
		}
	}
	return false
}

func classify(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, netguard.ErrBlockedAddress):
		return ReasonBlocked
	case errors.Is(err, errTooManyRedirects):
		return ReasonRedirects
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return ReasonTimeout
	}
	return ReasonConnect
}

// ValidateTarget refuses, at configuration time, a target every probe would refuse.
func ValidateTarget(ctx context.Context, m Monitor) error {
	if netguard.AllowPrivate() {
		return nil
	}
	if m.Kind == KindHTTP {
		if err := netguard.ValidateURL(ctx, m.Spec.URL); err != nil {
			return fmt.Errorf("%w: url target refused: %s", ErrInvalid, err.Error())
		}
		return nil
	}
	h := strings.ToLower(m.Spec.Host)
	if addr, err := netip.ParseAddr(h); err == nil && netguard.IsBlocked(addr) || h == "localhost" {
		return fmt.Errorf("%w: host is an internal address", ErrInvalid)
	}
	return nil
}
