package heartbeat

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

const maxRedirects = 3

// NewClient is the guarded HTTP client for off-host pings: no proxy, no private destinations unless allowed, same-host redirects only.
func NewClient(allowPrivate bool, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         guardedDialer(allowPrivate, timeout).DialContext,
			TLSHandshakeTimeout: timeout,
			DisableKeepAlives:   true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errors.New("too many redirects")
			}
			if !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) || (via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
				return errors.New("redirect to another host refused")
			}
			return nil
		},
	}
}

// Scrub drops the URL net/http wraps into its errors because it holds the monitor's secret key.
func Scrub(err error) error { return scrub(err) }
