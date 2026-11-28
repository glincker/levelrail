package dockerguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dockerclient "github.com/moby/moby/client"

	"github.com/moby/moby/api/types/container"
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
	upstream upstreamLog
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
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			g.upstream.ok(logger)
			return normalizeUpgrade(resp)
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			g.upstream.fail(logger, r, err)
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
		var req dockerclient.ExecCreateOptions
		if err := json.Unmarshal(raw, &req); err != nil {
			return invalid(err)
		}
		return reencode(req, validateExec(req))
	case bodyVolumeCreate:
		var req dockerclient.VolumeCreateOptions
		if err := json.Unmarshal(raw, &req); err != nil {
			return invalid(err)
		}
		return reencode(req, validateVolume(req))
	}
	return nil, nil
}

// jsonErrorKind describes a decode error without echoing body content.
func jsonErrorKind(err error) string {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntaxErr):
		return "syntax error at offset " + strconv.FormatInt(syntaxErr.Offset, 10)
	case errors.As(err, &typeErr):
		return "wrong type for field " + typeErr.Field
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

// normalizeUpgrade rewrites a 101 reply whose Upgrade value is a Docker
// stream media type (Podman does this) to the protocol the client asked for,
// because ReverseProxy refuses a reply that names a different protocol.
func normalizeUpgrade(res *http.Response) error {
	if res.StatusCode != http.StatusSwitchingProtocols {
		return nil
	}
	want := res.Request.Header.Get("Upgrade")
	if want == "" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(res.Header.Get("Upgrade"))) {
	case "application/vnd.docker.raw-stream", "application/vnd.docker.multiplexed-stream", "application/vnd.docker.stdin-stream":
		res.Header.Set("Upgrade", want)
		res.Header.Set("Connection", "Upgrade")
	}
	return nil
}

// upstreamWarnInterval spaces repeated identical "daemon unreachable" warnings, so a Mac
// with Docker switched off does not flood the service log with one line per poll.
const upstreamWarnInterval = 5 * time.Minute

// upstreamLog logs the first upstream failure, then at most one per interval, and a recovery line.
type upstreamLog struct {
	mu         sync.Mutex
	failing    bool
	lastWarn   time.Time
	suppressed int
	now        func() time.Time
}

func (u *upstreamLog) clock() time.Time {
	if u.now != nil {
		return u.now()
	}
	return time.Now()
}

func (u *upstreamLog) fail(logger *slog.Logger, r *http.Request, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := u.clock()
	if u.failing && now.Sub(u.lastWarn) < upstreamWarnInterval {
		u.suppressed++
		return
	}
	u.failing = true
	u.lastWarn = now
	logger.Warn("dockerguard: upstream request failed", slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.String("error", err.Error()), slog.Int("suppressed", u.suppressed))
	u.suppressed = 0
}

func (u *upstreamLog) ok(logger *slog.Logger) {
	u.mu.Lock()
	was := u.failing
	u.failing = false
	u.suppressed = 0
	u.mu.Unlock()
	if was {
		logger.Info("dockerguard: upstream docker daemon reachable again")
	}
}
