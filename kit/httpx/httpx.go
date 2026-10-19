// Package httpx holds bounded body reads and a client with sane defaults.
package httpx

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	// DefaultMaxBody caps an API response read into memory.
	DefaultMaxBody int64 = 32 << 20
	// MaxDownload caps a file-style response (CSV export, deploy log) read into memory.
	MaxDownload int64 = 256 << 20
)

// ErrBodyTooLarge is returned when a body exceeds its read limit.
var ErrBodyTooLarge = errors.New("httpx: response body exceeds limit")

// ReadAll reads at most limit bytes from r and fails with ErrBodyTooLarge
// rather than silently truncating when there is more.
func ReadAll(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w (%d bytes)", ErrBodyTooLarge, limit)
	}
	return data, nil
}

// ReadBody is ReadAll with DefaultMaxBody.
func ReadBody(r io.Reader) ([]byte, error) {
	return ReadAll(r, DefaultMaxBody)
}

// NewClient returns a client with an overall timeout and a transport that
// bounds dial, TLS handshake and header waits.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: timeout,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          20,
			MaxIdleConnsPerHost:   4,
			ForceAttemptHTTP2:     true,
		},
	}
}
