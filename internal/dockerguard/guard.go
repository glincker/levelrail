package dockerguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
)

// Guard is the policy-enforcing reverse proxy. It is an http.Handler; Serve
// puts it on a Unix socket.
type Guard struct {
	mode     atomic.Value
	policy   Policy
	grants   *Grants
	tunables Tunables
	proxy    *httputil.ReverseProxy
	recorder *recorder
	stats    *Stats
	logger   *slog.Logger
}

// Config is what New needs.
type Config struct {
	Mode     Mode
	Upstream string // Unix socket path of the real daemon
	Policy   Policy
	Grants   *Grants
	Tunables Tunables
	// Sink receives every decision worth an audit row; may be nil.
	Sink   Sink
	Logger *slog.Logger
}

// New builds a Guard forwarding to cfg.Upstream.
func New(cfg Config) *Guard {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	g := &Guard{
		policy:   cfg.Policy,
		grants:   cfg.Grants,
		tunables: cfg.Tunables,
		stats:    newStats(),
		logger:   logger,
	}
	g.recorder = newRecorder(cfg.Sink, cfg.Tunables, logger)
	g.mode.Store(cfg.Mode)
	upstream := cfg.Upstream
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", upstream)
		},
		DisableCompression: true,
		ForceAttemptHTTP2:  false,
	}
	g.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "docker"
			pr.Out.Host = "docker"
		},
		Transport:     transport,
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Warn("dockerguard: upstream request failed", slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.String("error", err.Error()))
			writeDockerError(w, http.StatusBadGateway, "docker guard: upstream docker daemon unreachable")
		},
	}
	return g
}

// Mode returns the mode in force right now.
func (g *Guard) Mode() Mode {
	m, _ := g.mode.Load().(Mode)
	return m
}

// SetMode switches between audit and enforce without a restart. Off is
// treated as audit: the socket stays up until the next boot.
func (g *Guard) SetMode(m Mode) {
	if m == ModeOff {
		m = ModeAudit
	}
	g.mode.Store(m)
}

// Stats exposes the in-memory counters since boot.
func (g *Guard) Stats() *Stats { return g.stats }

// Close stops the async recorder after draining what it already queued.
func (g *Guard) Close() { g.recorder.close() }

func (g *Guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cp, err := canonicalize(r.URL.EscapedPath(), r.URL.Path)
	if err != nil {
		g.decide(w, r, Decision{Method: r.Method, Path: r.URL.Path, Violations: []Violation{violation(RulePathNoncanonical, "%s", err.Error())}}, nil)
		return
	}
	d := Decision{Method: r.Method, Path: cp.Path}
	ep, ok := matchEndpoint(r.Method, cp.Path)
	if !ok {
		d.Violations = []Violation{violation(RuleEndpointNotAllowed, "%s %s is not on the allowlist", r.Method, cp.Path)}
		g.decide(w, r, d, nil)
		return
	}
	d.Pattern = ep.Pattern
	r.URL.Path, r.URL.RawPath = cp.Forward(), ""

	if ep.Body == bodyImageCreate && r.URL.Query().Get("fromSrc") != "" {
		d.Violations = []Violation{violation(RuleImageImport, "image import from fromSrc is not allowed, pull only")}
	}
	var canonicalBody []byte
	if ep.Body != bodyNone && ep.Body != bodyImageCreate {
		raw, tooLarge, rerr := readLimited(r.Body, g.tunables.MaxBodyBytes)
		switch {
		case rerr != nil:
			writeDockerError(w, http.StatusBadRequest, "docker guard: read request body: "+rerr.Error())
			return
		case tooLarge:
			d.Violations = append(d.Violations, violation(RuleBodyTooLarge, "request body exceeds %d bytes", g.tunables.MaxBodyBytes))
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), r.Body))
		default:
			d.Container = normalizeName(r.URL.Query().Get("name"))
			var vs []Violation
			canonicalBody, vs = g.validateBody(ep.Body, raw, d.Container)
			d.Violations = append(d.Violations, vs...)
			setBody(r, raw)
		}
	}
	if g.decide(w, r, d, canonicalBody) {
		g.proxy.ServeHTTP(w, r)
	}
}

// decide records d and reports whether the request may proceed. In
// enforce mode an allowed body is replaced by its re-encoded form, so the
// daemon sees exactly the fields that were validated.
func (g *Guard) decide(w http.ResponseWriter, r *http.Request, d Decision, canonicalBody []byte) bool {
	mode := g.Mode()
	d.Mode, d.At = mode, time.Now()
	if len(d.Violations) == 0 {
		if mode == ModeEnforce && canonicalBody != nil {
			setBody(r, canonicalBody)
		}
		return true
	}
	d.Denied = mode == ModeEnforce
	g.stats.observe(d)
	g.recorder.record(r.Context(), d)
	if !d.Denied {
		return true
	}
	writeDockerError(w, http.StatusForbidden, fmt.Sprintf("docker guard denied this request (rule %s): %s", d.Rule(), d.Violations[0].Reason))
	return false
}

func (g *Guard) validateBody(kind bodyKind, raw []byte, name string) ([]byte, []Violation) {
	invalid := func(err error) ([]byte, []Violation) {
		return nil, []Violation{violation(RuleBodyInvalid, "request body is not valid JSON for this endpoint: %s", jsonErrorKind(err))}
	}
	switch kind {
	case bodyContainerCreate:
		var req container.CreateRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return invalid(err)
		}
		return reencode(req, g.policy.validateCreate(req, g.grants.lookup(name)))
	case bodyContainerUpdate:
		var req container.UpdateConfig
		if err := json.Unmarshal(raw, &req); err != nil {
			return invalid(err)
		}
		return reencode(req, validateUpdate(req))
	case bodyExecCreate:
		var req container.ExecOptions
		if err := json.Unmarshal(raw, &req); err != nil {
			return invalid(err)
		}
		return reencode(req, validateExec(req))
	case bodyVolumeCreate:
		var req volume.CreateOptions
		if err := json.Unmarshal(raw, &req); err != nil {
			return invalid(err)
		}
		return reencode(req, validateVolume(req))
	}
	return nil, nil
}

// jsonErrorKind describes a decode error without echoing body content.
func jsonErrorKind(err error) string {
	switch e := err.(type) {
	case *json.SyntaxError:
		return "syntax error at offset " + strconv.FormatInt(e.Offset, 10)
	case *json.UnmarshalTypeError:
		return "wrong type for field " + e.Field
	default:
		return "malformed body"
	}
}

func reencode(v any, vs []Violation) ([]byte, []Violation) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, append(vs, violation(RuleBodyInvalid, "request body could not be re-encoded"))
	}
	return b, vs
}

func readLimited(body io.Reader, limit int64) ([]byte, bool, error) {
	if body == nil || body == http.NoBody {
		return []byte{}, false, nil
	}
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, false, err
	}
	return raw, int64(len(raw)) > limit, nil
}

func setBody(r *http.Request, b []byte) {
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))
	r.Header.Set("Content-Length", strconv.Itoa(len(b)))
	r.TransferEncoding = nil
	r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
}

func writeDockerError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
