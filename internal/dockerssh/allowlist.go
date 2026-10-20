// Package dockerssh reads a remote Docker host over an SSH login without
// running anything on it: SSH is only a transport to the Docker socket, and
// a client-side allowlist lets nothing but read-only Engine API GETs out.
package dockerssh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
)

// ErrRequestForbidden is wrapped by every refusal of the read-only transport.
var ErrRequestForbidden = errors.New("dockerssh: request is not on the read-only allowlist")

// ForbiddenRequestError says which request the allowlist refused and why.
type ForbiddenRequestError struct {
	Method string
	Path   string
	Reason string
}

func (e *ForbiddenRequestError) Error() string {
	return fmt.Sprintf("%v: %s %s (%s)", ErrRequestForbidden, e.Method, e.Path, e.Reason)
}

func (e *ForbiddenRequestError) Unwrap() error { return ErrRequestForbidden }

const maxInspectBody = 8 << 20

const (
	kindStatic  = "static"
	kindInspect = "inspect"
	kindArchive = "archive"
)

var (
	versionPrefixRe = regexp.MustCompile(`^/v[0-9]+\.[0-9]+(/.*)$`)
	nameSegment     = `[A-Za-z0-9][A-Za-z0-9_.-]{0,254}`
	imageNameRe     = `[A-Za-z0-9][A-Za-z0-9_.:/@-]{0,254}`
	containerRe     = regexp.MustCompile(`^/containers/(` + nameSegment + `)/(json|archive)$`)
	volumeRe        = regexp.MustCompile(`^/volumes/(` + nameSegment + `)$`)
	networkRe       = regexp.MustCompile(`^/networks/(` + nameSegment + `)$`)
	imageRe         = regexp.MustCompile(`^/images/(` + imageNameRe + `)/(json|get)$`)
	staticPaths     = map[string]bool{
		"/_ping": true, "/version": true, "/info": true,
		"/containers/json": true, "/volumes": true, "/networks": true, "/images/json": true,
		"/services": true, "/tasks": true, "/nodes": true,
	}
)

// ReadOnlyTransport is an http.RoundTripper that refuses every request that
// is not an allowlisted read-only Docker Engine API GET, before it is sent.
type ReadOnlyTransport struct {
	inner http.RoundTripper
	mu    sync.Mutex
	// mounts maps a container reference (id or name) to the mount
	// destinations its own inspect reported.
	mounts map[string]map[string]bool
}

// NewReadOnlyTransport wraps inner with the read-only allowlist.
func NewReadOnlyTransport(inner http.RoundTripper) *ReadOnlyTransport {
	return &ReadOnlyTransport{inner: inner, mounts: map[string]map[string]bool{}}
}

type route struct {
	container string
	kind      string
}

// Check reports whether the request is allowed. It is pure apart from the
// mount destinations learned from earlier container inspects.
func (t *ReadOnlyTransport) Check(req *http.Request) error {
	_, err := t.classify(req)
	return err
}

func (t *ReadOnlyTransport) classify(req *http.Request) (route, error) {
	deny := func(reason string) (route, error) {
		return route{}, &ForbiddenRequestError{Method: req.Method, Path: req.URL.Path, Reason: reason}
	}
	if req.Method != http.MethodGet {
		return deny("only GET is allowed")
	}
	if req.Body != nil && req.Body != http.NoBody {
		return deny("a GET with a body is not allowed")
	}
	p := req.URL.Path
	if strings.ContainsAny(p, "%\\\x00") || strings.Contains(p, "..") || strings.Contains(p, "//") || (req.URL.RawPath != "" && req.URL.RawPath != p) {
		return deny("path is not canonical")
	}
	if m := versionPrefixRe.FindStringSubmatch(p); m != nil {
		p = m[1]
	}
	if staticPaths[p] {
		return route{kind: kindStatic}, nil
	}
	if m := containerRe.FindStringSubmatch(p); m != nil {
		if m[2] == "json" {
			return route{container: m[1], kind: kindInspect}, nil
		}
		return t.checkArchive(req, m[1], deny)
	}
	if volumeRe.MatchString(p) || networkRe.MatchString(p) || imageRe.MatchString(p) {
		return route{kind: kindStatic}, nil
	}
	return deny("endpoint is not allowlisted")
}

func (t *ReadOnlyTransport) checkArchive(req *http.Request, ref string, deny func(string) (route, error)) (route, error) {
	want := req.URL.Query().Get("path")
	if want == "" || !strings.HasPrefix(want, "/") || path.Clean(want) != want {
		return deny("archive path must be a clean absolute mount destination")
	}
	t.mu.Lock()
	known := t.mounts[ref][want]
	t.mu.Unlock()
	if !known {
		return deny("archive path is not a mount destination of that container's inspect")
	}
	return route{container: ref, kind: kindArchive}, nil
}

// RoundTrip refuses anything off the allowlist and learns mount
// destinations from allowed container inspects.
func (t *ReadOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt, err := t.classify(req)
	if err != nil {
		return nil, err
	}
	resp, err := t.inner.RoundTrip(req)
	if err != nil || rt.kind != kindInspect || resp.StatusCode != http.StatusOK {
		return resp, err
	}
	raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxInspectBody+1))
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, fmt.Errorf("read container inspect: %w", rerr)
	}
	if len(raw) > maxInspectBody {
		return nil, errors.New("container inspect response is too large")
	}
	t.learn(rt.container, raw)
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	resp.ContentLength = int64(len(raw))
	return resp, nil
}

func (t *ReadOnlyTransport) learn(ref string, raw []byte) {
	var c struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Mounts []struct {
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return
	}
	dests := map[string]bool{}
	for _, m := range c.Mounts {
		if strings.HasPrefix(m.Destination, "/") {
			dests[path.Clean(m.Destination)] = true
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range []string{ref, c.ID, strings.TrimPrefix(c.Name, "/")} {
		if k != "" {
			t.mounts[k] = dests
		}
	}
}
