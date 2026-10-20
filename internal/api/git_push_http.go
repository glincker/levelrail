package api

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

const (
	envGitPushMaxBytes          = "APP_GIT_PUSH_MAX_BYTES"
	envGitPushQuotaBytes        = "APP_GIT_PUSH_QUOTA_BYTES"
	envGitPushTimeout           = "APP_GIT_PUSH_TIMEOUT"
	envGitPushRatePerMinute     = "APP_GIT_PUSH_RATE_PER_MINUTE"
	defaultGitPushMaxBytes      = int64(256 << 20)
	defaultGitPushQuotaBytes    = int64(1 << 30)
	defaultGitPushTimeout       = 30 * time.Minute
	defaultGitPushRatePerMinute = 30

	gitReceivePackService = "git-receive-pack"
	gitPushMirrorScheme   = "push://local/"
	gitPushMaxLineBytes   = 500
)

var (
	errPushTooLarge = errors.New("push exceeds the size limit")
	// A fixed hash keeps the compare cost the same when the app has no remote.
	gitPushDummyHash = tokenhash.Sum("no-remote")
)

type gitPushLimitSet struct {
	maxPushBytes int64
	quotaBytes   int64
	timeout      time.Duration
}

func gitPushLimits() gitPushLimitSet {
	return gitPushLimitSet{
		maxPushBytes: envInt64Positive(envGitPushMaxBytes, defaultGitPushMaxBytes),
		quotaBytes:   envInt64Positive(envGitPushQuotaBytes, defaultGitPushQuotaBytes),
		timeout:      envDuration(envGitPushTimeout, defaultGitPushTimeout),
	}
}

func envInt64Positive(name string, def int64) int64 {
	if v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64); err == nil && v > 0 {
		return v
	}
	return def
}

func gitPushMirrorURL(name string) string { return gitPushMirrorScheme + name }

func gitPushText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, msg+"\n")
}

// gitPushAuth authenticates a git client with HTTP basic auth. The token is the
// password; the username is ignored. Every failure answers the same 401 so an
// unknown app, a revoked remote and a wrong token look alike.
func (rt *Router) gitPushAuth(w http.ResponseWriter, r *http.Request) (name string, rm *store.GitPushRemote, ok bool) {
	if !featureOn(experimental.GitPush) {
		gitPushText(w, http.StatusNotFound, "not found")
		return "", nil, false
	}
	rt.gitPush.init()
	name = strings.TrimSuffix(r.PathValue("repo"), ".git")
	key := clientIP(r) + "|" + name
	if allowed, retry := rt.gitPush.rate.allow(key); !allowed {
		writeRateLimited(w, retry)
		return "", nil, false
	}
	if allowed, retry := rt.gitPush.fails.allow(key); !allowed {
		writeRateLimited(w, retry)
		return "", nil, false
	}
	_, token, hasAuth := r.BasicAuth()
	if !hasAuth || token == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="git push"`)
		gitPushText(w, http.StatusUnauthorized, "authentication required: use the remote URL printed when git push was enabled, or rotate the token")
		return "", nil, false
	}
	if rt.gitPushStore == nil {
		gitPushText(w, http.StatusNotImplemented, "git push is not configured on this control plane")
		return "", nil, false
	}
	stored := gitPushDummyHash
	rm, err := rt.gitPushStore.GetGitPushRemote(r.Context(), name)
	switch {
	case err == nil:
		stored = rm.TokenHash
	case errors.Is(err, store.ErrGitPushRemoteNotFound):
		rm = nil
	default:
		rt.logger.Error("api: git push: load remote failed", slog.String("name", name), slog.String("error", err.Error()))
		gitPushText(w, http.StatusInternalServerError, "internal error")
		return "", nil, false
	}
	match := tokenhash.Matches(token, stored)
	if rm == nil || !match {
		rt.gitPush.fails.recordFailure(key)
		rt.logger.Warn("api: git push: authentication failed", slog.String("name", name), slog.String("remote_addr", clientIP(r)))
		w.Header().Set("WWW-Authenticate", `Basic realm="git push"`)
		gitPushText(w, http.StatusUnauthorized, "authentication failed: the token is wrong, revoked or belongs to another app")
		return "", nil, false
	}
	rt.gitPush.fails.recordSuccess(key)
	return name, rm, true
}

// handleGitInfoRefs advertises the receive-pack refs of an app's push mirror.
// Only pushes are served: clone and fetch answer 403.
func (rt *Router) handleGitInfoRefs(w http.ResponseWriter, r *http.Request) {
	name, _, ok := rt.gitPushAuth(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("service") != gitReceivePackService {
		gitPushText(w, http.StatusForbidden, "this remote only accepts pushes: clone and fetch are not supported")
		return
	}
	repo, _, release, err := rt.sourceCache.OpenMirror(gitPushMirrorURL(name))
	if err != nil {
		rt.logger.Error("api: git push: open mirror failed", slog.String("name", name), slog.String("error", err.Error()))
		gitPushText(w, http.StatusNotImplemented, "git push needs the source cache directory, which is not configured on this control plane")
		return
	}
	defer release()

	ar := packp.NewAdvRefs()
	ar.Prefix = [][]byte{[]byte("# service=" + gitReceivePackService), pktline.Flush}
	_ = ar.Capabilities.Set(capability.Agent, capability.DefaultAgent())
	_ = ar.Capabilities.Set(capability.ReportStatus)
	_ = ar.Capabilities.Set(capability.Sideband64k)
	_ = ar.Capabilities.Set(capability.OFSDelta)
	iter, err := repo.Storer.IterReferences()
	if err == nil {
		defer iter.Close()
		_ = iter.ForEach(func(ref *plumbing.Reference) error {
			if ref.Type() == plumbing.HashReference && ref.Name().IsBranch() {
				ar.References[ref.Name().String()] = ref.Hash()
			}
			return nil
		})
	}
	w.Header().Set("Content-Type", "application/x-"+gitReceivePackService+"-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	if err := ar.Encode(w); err != nil {
		rt.logger.Warn("api: git push: write advertisement failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// capReader fails with errPushTooLarge instead of silently truncating.
type capReader struct {
	r        io.Reader
	left     int64
	exceeded bool
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		var b [1]byte
		n, err := c.r.Read(b[:])
		if n > 0 {
			c.exceeded = true
			return 0, errPushTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

// pushStream writes the receive-pack response: progress lines the client prints
// as "remote:" and the final report. Lines are held until start, because the
// request body must be fully read before the first response byte. After the
// handler returns it drops writes.
type pushStream struct {
	mu      sync.Mutex
	w       http.ResponseWriter
	fl      http.Flusher
	mux     *sideband.Muxer
	started bool
	closed  bool
	pending []string
}

func newPushStream(w http.ResponseWriter, caps *capability.List) *pushStream {
	p := &pushStream{w: w}
	p.fl, _ = w.(http.Flusher)
	switch {
	case caps.Supports(capability.Sideband64k):
		p.mux = sideband.NewMuxer(sideband.Sideband64k, w)
	case caps.Supports(capability.Sideband):
		p.mux = sideband.NewMuxer(sideband.Sideband, w)
	}
	return p
}

func (p *pushStream) say(text string) {
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(l, "\r ")
		if l == "" {
			continue
		}
		l = secretmask.Mask(l)
		if len(l) > gitPushMaxLineBytes {
			l = l[:gitPushMaxLineBytes] + "..."
		}
		p.mu.Lock()
		if !p.started {
			p.pending = append(p.pending, l)
		} else if !p.closed {
			p.writeLine(l)
		}
		p.mu.Unlock()
	}
}

func (p *pushStream) writeLine(l string) {
	if p.mux == nil {
		return
	}
	if _, err := p.mux.WriteChannel(sideband.ProgressMessage, []byte(l+"\n")); err != nil {
		p.closed = true
		return
	}
	if p.fl != nil {
		p.fl.Flush()
	}
}

func (p *pushStream) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.w.Header().Set("Content-Type", "application/x-"+gitReceivePackService+"-result")
	p.w.Header().Set("Cache-Control", "no-cache")
	p.w.WriteHeader(http.StatusOK)
	p.started = true
	for _, l := range p.pending {
		p.writeLine(l)
	}
	p.pending = nil
}

func (p *pushStream) report(cmds []*packp.Command, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	status := "ok"
	if reason != "" {
		status = oneLine(reason)
	}
	rs := packp.NewReportStatus()
	rs.UnpackStatus = "ok"
	for _, c := range cmds {
		rs.CommandStatuses = append(rs.CommandStatuses, &packp.CommandStatus{ReferenceName: c.Name, Status: status})
	}
	if p.mux != nil {
		_ = rs.Encode(p.mux)
		_ = pktline.NewEncoder(p.w).Flush()
	} else {
		_ = rs.Encode(p.w)
	}
	if p.fl != nil {
		p.fl.Flush()
	}
}

func (p *pushStream) close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
	if len(s) > gitPushMaxLineBytes {
		s = s[:gitPushMaxLineBytes]
	}
	return s
}

// handleGitReceivePack ingests a push into the app's mirror and deploys it.
func (rt *Router) handleGitReceivePack(w http.ResponseWriter, r *http.Request) {
	name, rm, ok := rt.gitPushAuth(w, r)
	if !ok {
		return
	}
	if !rt.gitPush.acquire(name) {
		gitPushText(w, http.StatusConflict, "another push for this app is in progress, try again in a moment")
		return
	}
	defer rt.gitPush.release(name)

	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	lim := gitPushLimits()
	var src io.Reader = r.Body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			gitPushText(w, http.StatusBadRequest, "malformed push request")
			return
		}
		defer func() { _ = zr.Close() }()
		src = zr
	}
	limit := min(lim.maxPushBytes, lim.quotaBytes)
	cr := &capReader{r: src, left: limit}
	br := bufio.NewReader(cr)

	if head, _ := br.Peek(5); len(head) == 4 && string(head) == "0000" {
		w.Header().Set("Content-Type", "application/x-"+gitReceivePackService+"-result")
		w.WriteHeader(http.StatusOK)
		return
	}

	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(br); err != nil {
		if cr.exceeded {
			gitPushText(w, http.StatusRequestEntityTooLarge, tooLargeMessage(limit))
			return
		}
		gitPushText(w, http.StatusBadRequest, "malformed push request")
		return
	}
	if !req.Capabilities.Supports(capability.ReportStatus) || len(req.Commands) == 0 {
		gitPushText(w, http.StatusBadRequest, "this git client is too old for this remote")
		return
	}

	ws := newPushStream(w, req.Capabilities)
	defer ws.close()
	plan, rej := rt.prepareGitPush(r, ws, name, rm, req, br, cr, limit)
	if rej != nil && rej.http != 0 {
		gitPushText(w, rej.http, rej.reason)
		return
	}
	reason := ""
	if rej != nil {
		reason = rej.reason
	}
	ws.start()
	if plan != nil {
		defer plan.release()
		reason = rt.deployGitPush(r, ws, name, plan, gitPushLimits().timeout)
	}
	rt.recordGitPushOutcome(r, name, req.Commands[0], reason)
	ws.report(req.Commands, reason)
}

func tooLargeMessage(limit int64) string {
	return "push is larger than the " + strconv.FormatInt(limit>>20, 10) + " MiB limit (APP_GIT_PUSH_MAX_BYTES)"
}
