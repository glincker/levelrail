package dockerguard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"

	"github.com/glincker/levelrail/internal/docker"
)

// Profile is a fixed, narrow slice of the Engine API handed to one template
// service through its own socket. Everything outside it is denied.
type Profile string

const (
	// ProfileReadOnly lists, inspects, reads logs, stats and events of the app's own containers.
	ProfileReadOnly Profile = "read_only"
	// ProfileProject adds create, start, stop, restart and exec, still on the app's own containers.
	ProfileProject Profile = "project"
	// ProfileLogs reads containers and logs of every app on the node, with env, host paths and secret labels stripped.
	ProfileLogs Profile = "logs"
)

// Rule ids for the profile sockets. Public contract like the ids in rules.go.
const (
	RuleProfileEndpoint = "profile_endpoint_not_allowed"
	RuleProfileScope    = "profile_scope_violation"
	RuleProfileDisabled = "profile_disabled"
	RuleProfileCreate   = "profile_create_not_allowed"
)

// reservedLabelPrefix mirrors spec.ReservedLabelPrefix without importing spec.
const reservedLabelPrefix = "platform-reserved."

// minProfileAPIMinor is the oldest v1.N Engine API a profile socket serves.
const minProfileAPIMinor = 24

// GrantCapPrefix marks a docker_api grant inside a service's granted caps, so
// the grant travels with the existing per-service grant plumbing.
const GrantCapPrefix = "DOCKER_API:"

// GrantCap is the granted-cap token for profile.
func GrantCap(p Profile) string { return GrantCapPrefix + strings.ToUpper(string(p)) }

// IsGrantCap reports whether c is a docker_api grant token.
func IsGrantCap(c string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(c)), GrantCapPrefix)
}

// ProfileFromCaps returns the profile a service's granted caps carry.
func ProfileFromCaps(caps []string) (Profile, bool) {
	for _, c := range caps {
		if !IsGrantCap(c) {
			continue
		}
		p := Profile(strings.ToLower(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), GrantCapPrefix)))
		if p == ProfileReadOnly || p == ProfileProject || p == ProfileLogs {
			return p, true
		}
	}
	return "", false
}

// ProfileSpec says which profile a socket serves and which project it is scoped to.
type ProfileSpec struct {
	Profile Profile
	// Project is the value of LabelKey that a container must carry to be visible.
	Project  string
	LabelKey string
	// Service and Network pin what ProfileProject may create: container names
	// must start with "<Service>-dapi-" and attach only to Network.
	Service string
	Network string
}

// Validate refuses an incomplete spec so a socket can never serve unscoped.
func (s ProfileSpec) Validate() error {
	if s.Profile != ProfileReadOnly && s.Profile != ProfileProject && s.Profile != ProfileLogs {
		return fmt.Errorf("dockerguard: unknown profile %q", s.Profile)
	}
	if s.Project == "" || s.LabelKey == "" {
		return fmt.Errorf("dockerguard: a profile socket needs a project and a label key")
	}
	if s.Profile == ProfileProject && (s.Service == "" || s.Network == "") {
		return fmt.Errorf("dockerguard: the project profile needs a service and a network")
	}
	return nil
}

type scopeKind int

const (
	scopeNone scopeKind = iota
	scopeFilter
	scopeContainer
	scopeExec
	scopeCreate
)

type profileEndpoint struct {
	Method  string
	Pattern string
	Scope   scopeKind
	Body    bodyKind
	Project bool // only in ProfileProject
}

var profileAllowlist = []profileEndpoint{
	{http.MethodGet, "/_ping", scopeNone, bodyNone, false},
	{http.MethodHead, "/_ping", scopeNone, bodyNone, false},
	{http.MethodGet, "/version", scopeNone, bodyNone, false},
	{http.MethodGet, "/info", scopeNone, bodyNone, false},
	{http.MethodGet, "/events", scopeFilter, bodyNone, false},
	{http.MethodGet, "/containers/json", scopeFilter, bodyNone, false},
	{http.MethodGet, "/containers/*/json", scopeContainer, bodyNone, false},
	{http.MethodGet, "/containers/*/logs", scopeContainer, bodyNone, false},
	{http.MethodGet, "/containers/*/stats", scopeContainer, bodyNone, false},
	{http.MethodGet, "/containers/*/top", scopeContainer, bodyNone, false},

	{http.MethodPost, "/containers/create", scopeCreate, bodyContainerCreate, true},
	{http.MethodPost, "/containers/*/start", scopeContainer, bodyNone, true},
	{http.MethodPost, "/containers/*/stop", scopeContainer, bodyNone, true},
	{http.MethodPost, "/containers/*/restart", scopeContainer, bodyNone, true},
	{http.MethodPost, "/containers/*/exec", scopeContainer, bodyExecCreate, true},
	{http.MethodPost, "/exec/*/start", scopeExec, bodyNone, true},
	{http.MethodPost, "/exec/*/resize", scopeExec, bodyNone, true},
	{http.MethodGet, "/exec/*/json", scopeExec, bodyNone, true},
}

func (s ProfileSpec) match(method, path string) (profileEndpoint, bool) {
	for _, e := range profileAllowlist {
		if e.Method != method || !matchPattern(e.Pattern, path) {
			continue
		}
		if e.Project && s.Profile != ProfileProject {
			continue
		}
		// Process command lines can carry secrets, so logs never gets top.
		if s.Profile == ProfileLogs && e.Pattern == "/containers/*/top" {
			continue
		}
		// Daemon info is for log viewers (full) and read_only (counts only).
		if s.Profile == ProfileProject && e.Pattern == "/info" {
			continue
		}
		return e, true
	}
	return profileEndpoint{}, false
}

// ProfileGuard serves one profile on one socket. It never forwards a request
// it did not match, and a container is reachable only after an upstream
// inspect proves it carries the project label.
type ProfileGuard struct {
	spec    ProfileSpec
	policy  Policy
	enabled func() bool
	maxBody int64
	proxy   *httputil.ReverseProxy
	client  *http.Client
	logger  *slog.Logger
}

// NewProfileGuard builds the handler for spec in front of the daemon socket
// upstream. enabled is read per request so switching the runtime setting off
// revokes access at once; a nil enabled means denied.
func NewProfileGuard(upstream string, spec ProfileSpec, policy Policy, tun Tunables, enabled func() bool, logger *slog.Logger) (*ProfileGuard, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", upstream)
		},
		DisableCompression: true,
	}
	pg := &ProfileGuard{spec: spec, policy: policy, enabled: enabled, maxBody: tun.MaxBodyBytes, logger: logger,
		client: &http.Client{Transport: transport, Timeout: 10 * time.Second}}
	pg.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = "http", "docker", "docker"
		},
		Transport:      transport,
		FlushInterval:  -1,
		ModifyResponse: filterResponse,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			logger.Warn("dockerguard: profile upstream unreachable", slog.String("error", err.Error()))
			writeDockerError(w, http.StatusBadGateway, "docker guard: upstream docker daemon unreachable")
		},
	}
	return pg, nil
}

func (g *ProfileGuard) deny(w http.ResponseWriter, rule, reason string) {
	writeDockerError(w, http.StatusForbidden, fmt.Sprintf("docker guard denied this request (rule %s): %s", rule, reason))
}

func (g *ProfileGuard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.enabled == nil || !g.enabled() {
		g.deny(w, RuleProfileDisabled, "docker api access is switched off")
		return
	}
	cp, err := canonicalize(r.URL.EscapedPath(), r.URL.Path)
	if err != nil {
		g.deny(w, RulePathNoncanonical, err.Error())
		return
	}
	ep, ok := g.spec.match(r.Method, cp.Path)
	if !ok {
		g.deny(w, RuleProfileEndpoint, r.Method+" "+cp.Path+" is not part of the "+string(g.spec.Profile)+" profile")
		return
	}
	if !versionAtLeast(cp.Version, minProfileAPIMinor) {
		g.deny(w, RuleProfileEndpoint, "api versions below v1."+strconv.Itoa(minProfileAPIMinor)+" are not served")
		return
	}
	if ep.Body == bodyNone && ep.Pattern != "/exec/*/start" && hasBody(r) {
		g.deny(w, RuleBodyInvalid, "this endpoint takes no request body")
		return
	}
	r.URL.Path, r.URL.RawPath = cp.Forward(), ""
	segs := splitSegs(cp.Path)
	switch ep.Scope {
	case scopeFilter:
		f, ferr := scopeFilters(r.URL.Query().Get("filters"), g.spec.scopeCondition(), g.spec.Profile == ProfileLogs && ep.Pattern == "/events")
		if ferr != nil {
			g.deny(w, RuleBodyInvalid, "filters is not valid JSON")
			return
		}
		q := r.URL.Query()
		q.Set("filters", f)
		r.URL.RawQuery = q.Encode()
	case scopeContainer:
		if !g.ownsContainer(r.Context(), segs[1]) {
			g.deny(w, RuleProfileScope, "container is not part of this project")
			return
		}
	case scopeExec:
		if !g.ownsExec(r.Context(), segs[1]) {
			g.deny(w, RuleProfileScope, "exec is not part of this project")
			return
		}
	}
	if g.spec.Profile == ProfileLogs {
		r = r.WithContext(withRespFilter(r.Context(), logsFilterFor(ep.Pattern)))
		if ep.Pattern == "/containers/*/logs" {
			// details would echo log driver extra attributes.
			q := r.URL.Query()
			q.Del("details")
			r.URL.RawQuery = q.Encode()
		}
	} else if ep.Pattern == "/info" {
		r = r.WithContext(withRespFilter(r.Context(), respFilterInfoMin))
	}
	if ep.Body != bodyNone && !g.rewriteBody(w, r, ep) {
		return
	}
	g.proxy.ServeHTTP(w, r)
}

// rewriteBody validates the body, forces the project label on a create, and
// replaces the body with the re-encoded form the daemon will see.
func (g *ProfileGuard) rewriteBody(w http.ResponseWriter, r *http.Request, ep profileEndpoint) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, g.maxBody+1))
	if err != nil || int64(len(raw)) > g.maxBody {
		g.deny(w, RuleBodyTooLarge, "request body is unreadable or too large")
		return false
	}
	var vs []Violation
	var out []byte
	switch ep.Body {
	case bodyContainerCreate:
		var req container.CreateRequest
		if json.Unmarshal(raw, &req) != nil || req.Config == nil {
			g.deny(w, RuleBodyInvalid, "request body is not a container create")
			return false
		}
		// Checked on the client's labels before ours is forced in.
		vs = g.spec.validateProfileCreate(req, r.URL.Query()["name"])
		if req.Labels == nil {
			req.Labels = map[string]string{}
		}
		req.Labels[g.spec.LabelKey] = g.spec.Project
		vs = append(vs, g.policy.validateCreate(req, docker.CreateDeclaration{})...)
		out, vs = reencode(req, vs)
	case bodyExecCreate:
		var req container.ExecOptions
		if json.Unmarshal(raw, &req) != nil {
			g.deny(w, RuleBodyInvalid, "request body is not an exec create")
			return false
		}
		out, vs = reencode(req, validateExec(req))
	}
	if len(vs) > 0 {
		g.deny(w, vs[0].Rule, vs[0].Reason)
		return false
	}
	setBody(r, out)
	return true
}

// ownsContainer inspects ref upstream and checks the project label. Any
// failure answers false: an unknown container is never forwarded to.
func (g *ProfileGuard) ownsContainer(ctx context.Context, ref string) bool {
	var info struct {
		Config struct{ Labels map[string]string } `json:"Config"`
	}
	if !g.getJSON(ctx, "/containers/"+url.PathEscape(ref)+"/json", &info) {
		return false
	}
	if g.spec.Profile == ProfileLogs {
		// Any container that carries an app label; the control plane and foreign containers carry none.
		return info.Config.Labels[g.spec.LabelKey] != ""
	}
	return info.Config.Labels[g.spec.LabelKey] == g.spec.Project
}

// scopeCondition is the label filter every list and event call is pinned to.
func (s ProfileSpec) scopeCondition() string {
	if s.Profile == ProfileLogs {
		return s.LabelKey
	}
	return s.LabelKey + "=" + s.Project
}

func (g *ProfileGuard) ownsExec(ctx context.Context, id string) bool {
	var info struct {
		ContainerID string `json:"ContainerID"`
	}
	if !g.getJSON(ctx, "/exec/"+url.PathEscape(id)+"/json", &info) || info.ContainerID == "" {
		return false
	}
	return g.ownsContainer(ctx, info.ContainerID)
}

func (g *ProfileGuard) getJSON(ctx context.Context, path string, into any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return false
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(into) == nil
}

// scopeFilters returns the Engine filters JSON with the project label
// condition added. Other keys the client sent are kept; a label list is only
// ever widened by the project condition, never replaced away from it.
func scopeFilters(raw, cond string, containerEventsOnly bool) (string, error) {
	filters := map[string][]string{}
	if raw != "" {
		var list map[string][]string
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			filters = list
		} else {
			var set map[string]map[string]bool
			if err := json.Unmarshal([]byte(raw), &set); err != nil {
				return "", err
			}
			for k, m := range set {
				for v, on := range m {
					if on {
						filters[k] = append(filters[k], v)
					}
				}
			}
		}
	}
	if containerEventsOnly {
		filters["type"] = []string{"container"}
	}
	if !slices.Contains(filters["label"], cond) {
		filters["label"] = append(filters["label"], cond)
	}
	b, err := json.Marshal(filters)
	return string(b), err
}

func versionAtLeast(version string, minor int) bool {
	if version == "" {
		return true
	}
	maj, rest, ok := strings.Cut(strings.TrimPrefix(version, "v"), ".")
	m, err := strconv.Atoi(rest)
	return ok && err == nil && maj == "1" && m >= minor
}

func hasBody(r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return false
	}
	var one [1]byte
	n, _ := r.Body.Read(one[:])
	return n > 0
}

// validateProfileCreate holds a project-profile create to the app's own
// name prefix, labels and network, with no host binds or published ports.
func (s ProfileSpec) validateProfileCreate(req container.CreateRequest, names []string) []Violation {
	var out []Violation
	prefix := s.Service + "-dapi-"
	if len(names) != 1 || !strings.HasPrefix(names[0], prefix) || len(names[0]) == len(prefix) {
		out = append(out, violation(RuleProfileCreate, "container name must be given once and start with %s", prefix))
	}
	for k := range req.Labels {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(k)), reservedLabelPrefix) {
			out = append(out, violation(RuleProfileCreate, "labels under %s are reserved", reservedLabelPrefix))
			break
		}
	}
	if req.NetworkingConfig != nil {
		for name := range req.NetworkingConfig.EndpointsConfig {
			if !s.networkAllowed(name) {
				out = append(out, violation(RuleProfileCreate, "network %q is not this app's network", name))
			}
		}
	}
	hc := req.HostConfig
	if hc == nil {
		return out
	}
	if !s.networkAllowed(string(hc.NetworkMode)) {
		out = append(out, violation(RuleProfileCreate, "HostConfig.NetworkMode %q is not this app's network", hc.NetworkMode))
	}
	if len(hc.Binds) > 0 {
		out = append(out, violation(RuleProfileCreate, "HostConfig.Binds is not allowed through a docker api grant"))
	}
	for _, m := range hc.Mounts {
		if m.Type != mount.TypeTmpfs {
			out = append(out, violation(RuleProfileCreate, "Mounts type %q is not allowed through a docker api grant", m.Type))
		}
	}
	if len(hc.PortBindings) > 0 || hc.PublishAllPorts {
		out = append(out, violation(RuleProfileCreate, "published ports are not allowed through a docker api grant"))
	}
	return out
}

func (s ProfileSpec) networkAllowed(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "default", "bridge":
		return true
	}
	return name == s.Network
}
