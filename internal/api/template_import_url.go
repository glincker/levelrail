package api

import (
	"context"
	"crypto/tls"
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

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const templateImportAuditAction = "templates.import_url"

var (
	errTemplateURLScheme   = errors.New("url must use https")
	errTemplateURLBlocked  = errors.New("url resolves to a private or local address")
	errTemplateURLRedirect = errors.New("url redirects; give the final address")
	errTemplateURLTooLarge = errors.New("file is larger than the import size limit")
)

// templateFetchOpts carries the live limits; the extra fields exist so tests can reach a local TLS server.
type templateFetchOpts struct {
	MaxBytes     int
	Timeout      time.Duration
	AllowPrivate bool
	TLSConfig    *tls.Config
}

// blockedTemplateIP refuses every address a public file host never uses.
func blockedTemplateIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified()
}

// fetchTemplateImport downloads one https file. No credentials, redirects or proxies are used,
// and the body is returned only to the caller, never logged.
func fetchTemplateImport(ctx context.Context, rawURL string, o templateFetchOpts) (string, error) {
	if err := build.ValidatePublicRepoURL(rawURL); err != nil {
		return "", errTemplateURLScheme
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", errTemplateURLScheme
	}
	dialer := &net.Dialer{Timeout: o.Timeout, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			return splitErr
		}
		ip, parseErr := netip.ParseAddr(host)
		if parseErr != nil || (!o.AllowPrivate && blockedTemplateIP(ip)) {
			return errTemplateURLBlocked
		}
		return nil
	}}
	client := &http.Client{
		Timeout: o.Timeout,
		Transport: &http.Transport{
			Proxy: nil, DialContext: dialer.DialContext, TLSClientConfig: o.TLSConfig,
			TLSHandshakeTimeout: o.Timeout, ResponseHeaderTimeout: o.Timeout,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", errTemplateURLScheme
	}
	resp, err := client.Do(req) //nolint:gosec // https only, dial-time address check above
	if err != nil {
		if errors.Is(err, errTemplateURLBlocked) {
			return "", errTemplateURLBlocked
		}
		return "", errors.New("could not download the file")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return "", errTemplateURLRedirect
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(o.MaxBytes)+1))
	if err != nil {
		return "", errors.New("could not read the file")
	}
	if len(body) > o.MaxBytes {
		return "", errTemplateURLTooLarge
	}
	return string(body), nil
}

// loadTemplateImportURL fetches the file with the live import limits and audits the host only.
func (rt *Router) loadTemplateImportURL(r *http.Request, rawURL string) (string, error) {
	o := templateFetchOpts{MaxBytes: 256 << 10, Timeout: 10 * time.Second}
	if rt.runtimeSettings != nil {
		o.MaxBytes = rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyTemplatesImportMaxBytes)
		o.Timeout = rt.runtimeSettings.TemplatesImportTimeout(r.Context())
	}
	host := ""
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		host = u.Host
	}
	body, err := fetchTemplateImport(r.Context(), strings.TrimSpace(rawURL), o)
	if ptype, pid, _, perr := rt.callerPrincipal(r); perr == nil {
		status := http.StatusOK
		if err != nil {
			status = http.StatusUnprocessableEntity
		}
		rt.auditSignIn(r.Context(), r, signInActor{kind: ptype, id: pid, name: pid}, templateImportAuditAction,
			"/api/v1/templates/custom host="+host, status)
	}
	return body, err
}
