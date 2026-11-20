package backup

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// WebDAVEndpointScheme prefixes the stored endpoint of a WebDAV target so it fits the
// existing provider column; the remainder is the real https (or http) base URL.
const WebDAVEndpointScheme = store.WebDAVEndpointScheme

// webDAVAttempts and webDAVBackoff bound retries of idempotent requests and seekable uploads.
const webDAVAttempts = 3

var webDAVBackoff = 300 * time.Millisecond

// WebDAVSettings are the live WebDAV tunables; the API layer feeds them from runtime settings.
type WebDAVSettings struct {
	AllowPrivate                 bool
	HeaderTimeout, Idle, Request time.Duration
	MaxResponse                  int64
	MaxEntries, MaxDepth         int
}

func defaultWebDAVSettings() WebDAVSettings {
	return WebDAVSettings{HeaderTimeout: 30 * time.Second, Idle: time.Minute, Request: 2 * time.Minute,
		MaxResponse: 8 << 20, MaxEntries: 20000, MaxDepth: 8}
}

var webDAVSource atomic.Pointer[func() WebDAVSettings]

// SetWebDAVSource makes WebDAV targets follow live settings; unset, the safe defaults apply.
func SetWebDAVSource(f func() WebDAVSettings) { webDAVSource.Store(&f) }

func webDAVSettings() WebDAVSettings {
	if f := webDAVSource.Load(); f != nil {
		return (*f)()
	}
	return defaultWebDAVSettings()
}

// WebDAVAllowPrivate reports whether WebDAV targets may reach private networks.
func WebDAVAllowPrivate() bool { return webDAVSettings().AllowPrivate }

// ValidateWebDAVHost refuses internal destinations unless the dedicated opt-in is on.
func ValidateWebDAVHost(ctx context.Context, endpoint string) error {
	if WebDAVAllowPrivate() {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return ErrWebDAVBadEndpoint
	}
	host := strings.ToLower(u.Hostname())
	blocked := fmt.Errorf("%w: %s (enable the WebDAV private network setting to allow it)", netguard.ErrBlockedAddress, host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return blocked
	}
	if ip, perr := netip.ParseAddr(host); perr == nil {
		if netguard.IsBlocked(ip) {
			return blocked
		}
		return nil
	}
	lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, lerr := net.DefaultResolver.LookupNetIP(lctx, "ip", host)
	if lerr != nil || len(addrs) == 0 {
		return nil
	}
	for _, a := range addrs {
		if !netguard.IsBlocked(a) {
			return nil
		}
	}
	return blocked
}

// ValidateCABundle accepts only CERTIFICATE PEM blocks that parse as certificates.
func ValidateCABundle(bundle string) error {
	rest, n := []byte(bundle), 0
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			return errors.New("ca_bundle may only contain CERTIFICATE PEM blocks")
		}
		if _, err := x509.ParseCertificate(b.Bytes); err != nil {
			return errors.New("ca_bundle holds an invalid certificate")
		}
		n++
	}
	if n == 0 || len(strings.TrimSpace(string(rest))) != 0 {
		return errors.New("ca_bundle must be PEM encoded certificates")
	}
	return nil
}

// ErrWebDAVBadEndpoint means the stored WebDAV endpoint is not a usable URL.
var ErrWebDAVBadEndpoint = errors.New("backup: webdav endpoint must be an https URL")

// IsWebDAV reports whether the destination is a WebDAV share.
func (d Destination) IsWebDAV() bool { return strings.HasPrefix(d.Endpoint, WebDAVEndpointScheme) }

// TestReport is what a connection probe measured.
type TestReport struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms"`
	FreeBytes *int64 `json:"free_bytes,omitempty"`
}

// Prober is a Tester that also reports latency and free space.
type Prober interface {
	Probe(ctx context.Context, dest Destination) (TestReport, error)
}

type webDAV struct {
	base   *url.URL
	client *http.Client
	user   string
	pass   string
	// idle bounds the gap between body reads; request caps non-streaming calls.
	idle, request time.Duration
	set           WebDAVSettings
}

// WebDAVBaseURL turns a stored endpoint and directory into the collection URL.
func WebDAVBaseURL(endpoint, dir string) (*url.URL, error) {
	raw := strings.TrimPrefix(endpoint, WebDAVEndpointScheme)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, ErrWebDAVBadEndpoint
	}
	if u.Scheme == "http" && !WebDAVAllowPrivate() {
		return nil, ErrWebDAVBadEndpoint
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	u.Path = path.Join("/", u.Path, dir)
	return u, nil
}

func newWebDAV(dest Destination) (*webDAV, error) {
	base, err := WebDAVBaseURL(dest.Endpoint, dest.Bucket)
	if err != nil {
		return nil, err
	}
	c := netguard.NewClient()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		return nil, errors.New("backup: webdav transport unavailable")
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if dest.CABundle != "" {
		if err := ValidateCABundle(dest.CABundle); err != nil {
			return nil, fmt.Errorf("backup: webdav: %w", err)
		}
		pool.AppendCertsFromPEM([]byte(dest.CABundle))
	}
	set := webDAVSettings()
	tr.ResponseHeaderTimeout = set.HeaderTimeout
	// The dial check reads the setting per connection so a toggle applies without a new client.
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: davDialControl}
	tr.DialContext = dialer.DialContext
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &webDAV{base: base, client: c, user: dest.AccessKeyID, pass: dest.SecretAccessKey,
		idle: set.Idle, request: set.Request, set: set}, nil
}

func (w *webDAV) url(key string) string {
	u := *w.base
	u.Path = path.Join(u.Path, key)
	return u.String()
}

// guardedBody cancels the request when reads stall and releases it on Close.
type guardedBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	timer  *time.Timer
	idle   time.Duration
}

func (g *guardedBody) Read(p []byte) (int, error) {
	g.timer.Reset(g.idle)
	return g.ReadCloser.Read(p)
}

func (g *guardedBody) Close() error {
	g.timer.Stop()
	err := g.ReadCloser.Close()
	g.cancel()
	return err
}

func (w *webDAV) do(ctx context.Context, method, target string, body io.Reader, hdr map[string]string) (*http.Response, error) {
	var cancel context.CancelFunc
	// Bulk transfers get an idle deadline only; small verbs also get an overall cap.
	if method == http.MethodGet || method == http.MethodPut {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, w.request)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		cancel()
		return nil, err
	}
	req.SetBasicAuth(w.user, w.pass)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &guardedBody{ReadCloser: resp.Body, cancel: cancel, timer: time.AfterFunc(w.idle, cancel), idle: w.idle}
	return resp, nil
}

// retry runs fn up to webDAVAttempts times on transport errors and 5xx answers.
func (w *webDAV) retry(ctx context.Context, method, target string, hdr map[string]string, body []byte) (*http.Response, error) {
	var last error
	for i := 0; i < webDAVAttempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(webDAVBackoff * time.Duration(i)):
			}
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		resp, err := w.do(ctx, method, target, rd, hdr)
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode >= 500 {
			last = fmt.Errorf("%s %s: %s", method, "webdav", resp.Status)
			_ = resp.Body.Close()
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("backup: webdav %s failed after %d attempts: %w", method, webDAVAttempts, last)
}

func statusErr(op string, resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("backup: webdav %s: authentication rejected (%s)", op, resp.Status)
	}
	return fmt.Errorf("backup: webdav %s: unexpected %s", op, resp.Status)
}

// mkcolAll creates every collection from the base down to dir, tolerating existing ones.
func (w *webDAV) mkcolAll(ctx context.Context, dir string) error {
	segs := []string{""}
	if dir = strings.Trim(dir, "/"); dir != "" {
		segs = append(segs, strings.Split(dir, "/")...)
	}
	cur := ""
	for _, s := range segs {
		cur = path.Join(cur, s)
		resp, err := w.retry(ctx, "MKCOL", w.url(cur), nil, nil)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		// 405 means the collection already exists, 301 is a server normalizing the slash.
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusMethodNotAllowed &&
			resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusOK {
			return statusErr("mkcol", resp)
		}
	}
	return nil
}

func validWebDAVKey(key string) error {
	if key == "" || strings.Contains(key, "\x00") {
		return errors.New("backup: webdav key is empty or invalid")
	}
	for _, s := range strings.Split(key, "/") {
		if s == ".." {
			return errors.New("backup: webdav key must not contain '..'")
		}
	}
	return nil
}

// Upload writes to a temporary name and moves it into place so a half written
// object never appears under key. A reader that can seek is retried whole.
func (w *webDAV) Upload(ctx context.Context, key string, r io.Reader) error {
	if err := validWebDAVKey(key); err != nil {
		return err
	}
	if err := w.mkcolAll(ctx, path.Dir(key)); err != nil {
		return err
	}
	tmp := key + ".partial"
	seeker, canSeek := r.(io.ReadSeeker)
	cr := &countReader{r: r}
	var last error
	for i := 0; i < webDAVAttempts; i++ {
		if i > 0 {
			if !canSeek {
				break
			}
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(webDAVBackoff * time.Duration(i)):
			}
		}
		cr.n = 0
		resp, err := w.do(ctx, http.MethodPut, w.url(tmp), cr, nil)
		if err != nil {
			last = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 {
			last = fmt.Errorf("put: %s", resp.Status)
			continue
		}
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			return statusErr("put", resp)
		}
		last = nil
		break
	}
	if last != nil {
		_ = w.Delete(context.WithoutCancel(ctx), tmp)
		return fmt.Errorf("backup: webdav upload: %w", last)
	}
	// MOVE is sent once: a blind retry after a lost answer would fail on the vanished source.
	resp, err := w.do(ctx, "MOVE", w.url(tmp), nil, map[string]string{"Destination": w.url(key), "Overwrite": "T"})
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			return nil
		}
	}
	if w.landed(context.WithoutCancel(ctx), key, cr.n) {
		return nil
	}
	_ = w.Delete(context.WithoutCancel(ctx), tmp)
	if err != nil {
		return fmt.Errorf("backup: webdav move: %w", err)
	}
	return statusErr("move", resp)
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// landed reports whether key already exists with the size we uploaded.
func (w *webDAV) landed(ctx context.Context, key string, size int64) bool {
	resp, err := w.do(ctx, http.MethodHead, w.url(key), nil, nil)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK && resp.ContentLength == size
}

// Download opens key for reading; a ranged read resumes a broken transfer.
func (w *webDAV) Download(ctx context.Context, key string, offset int64) (io.ReadCloser, error) {
	if err := validWebDAVKey(key); err != nil {
		return nil, err
	}
	hdr := map[string]string{}
	if offset > 0 {
		hdr["Range"] = "bytes=" + strconv.FormatInt(offset, 10) + "-"
	}
	resp, err := w.retry(ctx, http.MethodGet, w.url(key), hdr, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		return nil, statusErr("get", resp)
	}
	return resp.Body, nil
}

// Delete removes key; an already missing object counts as deleted.
func (w *webDAV) Delete(ctx context.Context, key string) error {
	if err := validWebDAVKey(key); err != nil {
		return err
	}
	resp, err := w.retry(ctx, http.MethodDelete, w.url(key), nil, nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound, http.StatusAccepted:
		return nil
	}
	return statusErr("delete", resp)
}

type davMultistatus struct {
	Responses []struct {
		Href  string `xml:"href"`
		Props []struct {
			Length    string    `xml:"prop>getcontentlength"`
			Available string    `xml:"prop>quota-available-bytes"`
			IsColl    *struct{} `xml:"prop>resourcetype>collection"`
		} `xml:"propstat"`
	} `xml:"response"`
}

type davEntry struct {
	name string
	dir  bool
	size int64
	free *int64
}

func (w *webDAV) propfind(ctx context.Context, rel string) ([]davEntry, error) {
	const body = `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getcontentlength/><d:resourcetype/><d:quota-available-bytes/></d:prop></d:propfind>`
	resp, err := w.retry(ctx, "PROPFIND", w.url(rel), map[string]string{"Depth": "1", "Content-Type": "application/xml"}, []byte(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, statusErr("propfind", resp)
	}
	var ms davMultistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, w.set.MaxResponse)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("backup: webdav propfind: bad response: %w", err)
	}
	selfPath := strings.TrimRight(path.Join(w.base.Path, rel), "/")
	var out []davEntry
	for _, r := range ms.Responses {
		href := r.Href
		if u, err := url.Parse(href); err == nil {
			href = u.Path
		}
		clean := strings.TrimRight(path.Clean("/"+href), "/")
		isSelf := clean == selfPath
		// Only the collection itself and its direct children are trusted.
		if !isSelf {
			parent := path.Dir(clean)
			if parent == "/" {
				parent = ""
			}
			if parent != selfPath {
				continue
			}
		}
		e := davEntry{name: path.Base(clean)}
		if e.name == "." || e.name == ".." || e.name == "/" {
			e.name = ""
		}
		for _, p := range r.Props {
			if p.IsColl != nil {
				e.dir = true
			}
			if n, err := strconv.ParseInt(strings.TrimSpace(p.Length), 10, 64); err == nil {
				e.size = n
			}
			if n, err := strconv.ParseInt(strings.TrimSpace(p.Available), 10, 64); err == nil {
				v := n
				e.free = &v
			}
		}
		if isSelf {
			e.name = ""
		}
		out = append(out, e)
	}
	return out, nil
}

// List returns every object under prefix with keys relative to the target directory.
func (w *webDAV) List(ctx context.Context, prefix string) ([]RemoteObject, error) {
	dir, namePrefix := path.Split(prefix)
	var out []RemoteObject
	budget := w.set.MaxEntries
	spend := func() error {
		if budget--; budget < 0 {
			return errors.New("backup: webdav listing too large")
		}
		return nil
	}
	var walk func(rel string, depth int) error
	walk = func(rel string, depth int) error {
		if err := spend(); err != nil {
			return err
		}
		entries, err := w.propfind(ctx, rel)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.name == "" || e.name == "." || e.name == ".." {
				continue
			}
			if err := spend(); err != nil {
				return err
			}
			key := path.Join(rel, e.name)
			if e.dir {
				if depth < w.set.MaxDepth {
					if err := walk(key, depth+1); err != nil {
						return err
					}
				}
				continue
			}
			if !strings.HasPrefix(key, dir+namePrefix) || strings.HasSuffix(key, ".partial") {
				continue
			}
			out = append(out, RemoteObject{Key: key, Size: e.size})
		}
		return nil
	}
	if err := walk(strings.Trim(dir, "/"), 0); err != nil {
		return nil, err
	}
	return out, nil
}

// Probe writes, reads back and deletes a tiny object, timing the round trip.
func (w *webDAV) Probe(ctx context.Context) (TestReport, error) {
	start := time.Now()
	key := ".levelrail-probe-" + strconv.FormatInt(start.UnixNano(), 36)
	if err := w.Upload(ctx, key, strings.NewReader("ok")); err != nil {
		return TestReport{}, err
	}
	rc, err := w.Download(ctx, key, 0)
	if err != nil {
		_ = w.Delete(ctx, key)
		return TestReport{}, err
	}
	got, readErr := io.ReadAll(io.LimitReader(rc, 64))
	_ = rc.Close()
	if readErr != nil || string(got) != "ok" {
		_ = w.Delete(ctx, key)
		return TestReport{}, errors.New("backup: webdav test: object read back did not match what was written")
	}
	if err := w.Delete(ctx, key); err != nil {
		return TestReport{}, err
	}
	rep := TestReport{OK: true, LatencyMs: time.Since(start).Milliseconds()}
	if entries, err := w.propfind(ctx, ""); err == nil && len(entries) > 0 {
		for _, e := range entries {
			if e.name == "" {
				rep.FreeBytes = e.free
			}
		}
	}
	return rep, nil
}

// davDialControl refuses blocked addresses at connect time unless the live opt-in is on.
func davDialControl(_, address string, _ syscall.RawConn) error {
	if WebDAVAllowPrivate() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("backup: webdav: parse dial address %q: %w", address, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("backup: webdav: parse dial address %q: %w", address, err)
	}
	if netguard.IsBlocked(ip) {
		return fmt.Errorf("%w: %s (enable the WebDAV private network setting to allow it)", netguard.ErrBlockedAddress, ip)
	}
	return nil
}
