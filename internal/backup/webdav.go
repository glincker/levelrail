package backup

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
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

// webDAVMaxList bounds a recursive listing so a hostile server cannot make us walk forever.
const webDAVMaxList = 20000

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
}

// WebDAVBaseURL turns a stored endpoint and directory into the collection URL.
func WebDAVBaseURL(endpoint, dir string) (*url.URL, error) {
	raw := strings.TrimPrefix(endpoint, WebDAVEndpointScheme)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, ErrWebDAVBadEndpoint
	}
	if u.Scheme == "http" && !netguard.AllowPrivate() {
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
	if dest.CABundle != "" && !pool.AppendCertsFromPEM([]byte(dest.CABundle)) {
		return nil, errors.New("backup: webdav ca bundle has no valid PEM certificate")
	}
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &webDAV{base: base, client: c, user: dest.AccessKeyID, pass: dest.SecretAccessKey}, nil
}

func (w *webDAV) url(key string) string {
	u := *w.base
	u.Path = path.Join(u.Path, key)
	return u.String()
}

func (w *webDAV) do(ctx context.Context, method, target string, body io.Reader, hdr map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(w.user, w.pass)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return w.client.Do(req)
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
		resp, err := w.do(ctx, http.MethodPut, w.url(tmp), r, nil)
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
	resp, err := w.retry(ctx, "MOVE", w.url(tmp), map[string]string{"Destination": w.url(key), "Overwrite": "T"}, nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		_ = w.Delete(context.WithoutCancel(ctx), tmp)
		return statusErr("move", resp)
	}
	return nil
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
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("backup: webdav propfind: bad response: %w", err)
	}
	selfPath := strings.TrimRight(path.Join(w.base.Path, rel), "/")
	var out []davEntry
	for _, r := range ms.Responses {
		href := r.Href
		if u, err := url.Parse(href); err == nil {
			href = u.Path
		}
		e := davEntry{name: path.Base(strings.TrimRight(href, "/"))}
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
		if strings.TrimRight(path.Clean("/"+href), "/") == selfPath {
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
	var walk func(rel string, depth int) error
	walk = func(rel string, depth int) error {
		entries, err := w.propfind(ctx, rel)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.name == "" {
				continue
			}
			key := path.Join(rel, e.name)
			if e.dir {
				if depth < 8 {
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
			if len(out) >= webDAVMaxList {
				return errors.New("backup: webdav listing too large")
			}
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
