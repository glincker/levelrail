package backup

import (
	"errors"

	"context"
	"encoding/pem"
	"fmt"
	"github.com/GLINCKER/levelrail/kit/netguard"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDAV is a tiny in-memory WebDAV server: enough verbs for the backup target.
type fakeDAV struct {
	mu      sync.Mutex
	files   map[string]string
	dirs    map[string]bool
	flaky   int
	user    string
	pass    string
	putSeen []string
}

func newFakeDAV() *fakeDAV {
	return &fakeDAV{files: map[string]string{}, dirs: map[string]bool{"/": true}, user: "u", pass: "p"}
}

func (f *fakeDAV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if u, p, ok := r.BasicAuth(); !ok || u != f.user || p != f.pass {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p := path.Clean(r.URL.Path)
	switch r.Method {
	case "MKCOL":
		if f.dirs[p] {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		f.dirs[p] = true
		w.WriteHeader(http.StatusCreated)
	case http.MethodPut:
		if f.flaky > 0 {
			f.flaky--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.files[p] = string(b)
		f.putSeen = append(f.putSeen, p)
		w.WriteHeader(http.StatusCreated)
	case "MOVE":
		dst := r.Header.Get("Destination")
		i := strings.Index(dst[8:], "/") + 8
		f.files[path.Clean(dst[i:])] = f.files[p]
		delete(f.files, p)
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		v, ok := f.files[p]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, v)
	case http.MethodDelete:
		delete(f.files, p)
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		var keys []string
		for k := range f.files {
			if path.Dir(k) == p {
				keys = append(keys, k)
			}
		}
		for k := range f.dirs {
			if k != p && path.Dir(k) == p {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`)
		fmt.Fprintf(&b, `<d:response><d:href>%s/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype><d:quota-available-bytes>12345</d:quota-available-bytes></d:prop></d:propstat></d:response>`, p)
		for _, k := range keys {
			if f.dirs[k] {
				fmt.Fprintf(&b, `<d:response><d:href>%s/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>`, k)
				continue
			}
			fmt.Fprintf(&b, `<d:response><d:href>%s</d:href><d:propstat><d:prop><d:getcontentlength>%d</d:getcontentlength></d:prop></d:propstat></d:response>`, k, len(f.files[k]))
		}
		b.WriteString(`</d:multistatus>`)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, b.String())
	}
}

// setWebDAVPrivate flips the live opt-in for one test and restores the defaults after.
func setWebDAVPrivate(t *testing.T, allow bool) {
	t.Helper()
	SetWebDAVSource(func() WebDAVSettings {
		d := defaultWebDAVSettings()
		d.AllowPrivate = allow
		return d
	})
	t.Cleanup(func() { SetWebDAVSource(defaultWebDAVSettings) })
}

func davDest(t *testing.T, srv *httptest.Server) Destination {
	t.Helper()
	setWebDAVPrivate(t, true)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	return Destination{Endpoint: WebDAVEndpointScheme + srv.URL, Bucket: "backups", AccessKeyID: "u", SecretAccessKey: "p", CABundle: string(ca)}
}

func TestWebDAVRoundTripAndProbe(t *testing.T) {
	webDAVBackoff = 0
	f := newFakeDAV()
	srv := httptest.NewTLSServer(f)
	defer srv.Close()
	dest := davDest(t, srv)
	ctx := context.Background()
	r := Routed{}

	if err := r.Upload(ctx, dest, "db/a/x.dump", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("upload: %v", err)
	}
	for _, p := range f.putSeen {
		if !strings.HasSuffix(p, ".partial") {
			t.Fatalf("object written without a temp name: %s", p)
		}
	}
	rc, err := r.Download(ctx, dest, "db/a/x.dump")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	objs, err := r.List(ctx, dest, "db/")
	if err != nil || len(objs) != 1 || objs[0].Key != "db/a/x.dump" || objs[0].Size != 5 {
		t.Fatalf("list = %+v, %v", objs, err)
	}
	if err := r.Delete(ctx, dest, "db/a/x.dump"); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, dest, "db/a/x.dump"); err != nil {
		t.Fatalf("second delete must succeed: %v", err)
	}
	rep, err := r.Probe(ctx, dest)
	if err != nil || !rep.OK || rep.FreeBytes == nil || *rep.FreeBytes != 12345 {
		t.Fatalf("probe = %+v, %v", rep, err)
	}
	if len(f.files) != 0 {
		t.Fatalf("probe left files behind: %v", f.files)
	}
}

func TestWebDAVRefusesUnknownCAAndBadCredentials(t *testing.T) {
	f := newFakeDAV()
	srv := httptest.NewTLSServer(f)
	defer srv.Close()
	dest := davDest(t, srv)
	ctx := context.Background()

	noCA := dest
	noCA.CABundle = ""
	if err := (Routed{}).Test(ctx, noCA); err == nil {
		t.Fatal("a self signed server must be refused without its CA bundle")
	}
	bad := dest
	bad.SecretAccessKey = "wrong"
	err := (Routed{}).Test(ctx, bad)
	if err == nil || !strings.Contains(err.Error(), "authentication rejected") {
		t.Fatalf("want auth rejected, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Fatal("password leaked into the error")
	}
}

func TestWebDAVBlocksPrivateHostByDefault(t *testing.T) {
	srv := httptest.NewTLSServer(newFakeDAV())
	defer srv.Close()
	dest := davDest(t, srv)
	setWebDAVPrivate(t, false)
	if err := (Routed{}).Test(context.Background(), dest); !errors.Is(err, netguard.ErrBlockedAddress) {
		t.Fatalf("loopback must be refused, got %v", err)
	}
}

func TestWebDAVRetriesTransientFailure(t *testing.T) {
	webDAVBackoff = 0
	f := newFakeDAV()
	f.flaky = 2
	srv := httptest.NewTLSServer(f)
	defer srv.Close()
	dest := davDest(t, srv)
	if err := (Routed{}).Upload(context.Background(), dest, "k", strings.NewReader("v"), 1); err != nil {
		t.Fatalf("seekable upload should survive two 503s: %v", err)
	}
}

func TestWebDAVRejectsTraversalKey(t *testing.T) {
	srv := httptest.NewTLSServer(newFakeDAV())
	defer srv.Close()
	dest := davDest(t, srv)
	if err := (Routed{}).Upload(context.Background(), dest, "../x", strings.NewReader("v"), 1); err == nil {
		t.Fatal("traversal key accepted")
	}
}

func TestWebDAVListIgnoresStrayHrefs(t *testing.T) {
	webDAVBackoff = 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		const resp = `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">
<d:response><d:href>/backups/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
<d:response><d:href>/backups/good.dump</d:href><d:propstat><d:prop><d:getcontentlength>3</d:getcontentlength></d:prop></d:propstat></d:response>
<d:response><d:href>/backups/../escape.dump</d:href><d:propstat><d:prop><d:getcontentlength>9</d:getcontentlength></d:prop></d:propstat></d:response>
<d:response><d:href>/backups/sub/deep.dump</d:href><d:propstat><d:prop><d:getcontentlength>9</d:getcontentlength></d:prop></d:propstat></d:response>
<d:response><d:href>/other/x.dump</d:href><d:propstat><d:prop><d:getcontentlength>9</d:getcontentlength></d:prop></d:propstat></d:response>
<d:response><d:href>/backups/.</d:href><d:propstat><d:prop><d:getcontentlength>9</d:getcontentlength></d:prop></d:propstat></d:response>
</d:multistatus>`
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()
	dest := davDest(t, srv)
	dest.AccessKeyID, dest.SecretAccessKey = "", ""
	objs, err := (Routed{}).List(context.Background(), dest, "")
	if err != nil || len(objs) != 1 || objs[0].Key != "good.dump" {
		t.Fatalf("list = %+v, %v", objs, err)
	}
}

func TestWebDAVMoveIsSentOnce(t *testing.T) {
	webDAVBackoff = 0
	var moves, heads int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
		case "MOVE":
			moves++
			w.WriteHeader(http.StatusInternalServerError)
		case http.MethodHead:
			heads++
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	dest := davDest(t, srv)
	dest.AccessKeyID, dest.SecretAccessKey = "", ""
	if err := (Routed{}).Upload(context.Background(), dest, "k", strings.NewReader("v"), 1); err == nil {
		t.Fatal("a failed move with no object at the destination must fail")
	}
	if moves != 1 || heads != 1 {
		t.Fatalf("moves=%d heads=%d, want 1 and 1", moves, heads)
	}
}

func TestWebDAVMoveErrorButObjectLanded(t *testing.T) {
	webDAVBackoff = 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
		case "MOVE":
			w.WriteHeader(http.StatusBadGateway)
		case http.MethodHead:
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()
	dest := davDest(t, srv)
	if err := (Routed{}).Upload(context.Background(), dest, "k", strings.NewReader("v"), 1); err != nil {
		t.Fatalf("object landed, upload should succeed: %v", err)
	}
}

func TestWebDAVHeaderTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	dest := davDest(t, srv)
	SetWebDAVSource(func() WebDAVSettings {
		d := defaultWebDAVSettings()
		d.AllowPrivate, d.HeaderTimeout = true, 150*time.Millisecond
		return d
	})
	if _, err := (Routed{}).Download(context.Background(), dest, "k"); err == nil {
		t.Fatal("a server that never answers must time out")
	}
}

func TestWebDAVMetadataStaysBlockedWithPrivateOptIn(t *testing.T) {
	setWebDAVPrivate(t, true)
	for _, ep := range []string{"http://169.254.169.254/", "http://[fe80::1]/", "http://[fd00:ec2::254]/", "http://metadata.google.internal/"} {
		if err := ValidateWebDAVHost(context.Background(), ep); !errors.Is(err, netguard.ErrBlockedAddress) {
			t.Fatalf("%s must be refused, got %v", ep, err)
		}
	}
	if err := davDialControl("tcp", "169.254.169.254:80", nil); !errors.Is(err, netguard.ErrBlockedAddress) {
		t.Fatalf("dial to metadata must be refused, got %v", err)
	}
	if err := ValidateWebDAVHost(context.Background(), "http://192.168.1.5/"); err != nil {
		t.Fatalf("plain private host must be allowed: %v", err)
	}
}
