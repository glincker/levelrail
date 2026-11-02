package api

import (
	"compress/gzip"
	"context"
	"crypto/subtle"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/erroringest"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	// maxSentryBody bounds one request after decompression.
	maxSentryBody = 1 << 20
	// sentryProjectID is the fixed project segment of the DSN: one control
	// plane has no projects, the key alone names the app.
	sentryProjectID = "1"

	envErrorIngestPerMin    = "APP_ERROR_INGEST_PER_MIN"
	envErrorEventsPerIssue  = "APP_ERROR_EVENTS_PER_ISSUE"
	defaultErrorIngestRate  = 600
	deployIDCacheTTL        = 10 * time.Second
	errorIngestSecretName   = "key"
	errorIngestSecretPrefix = "error-ingest/"
)

func errorIngestSecretsKey(app string) string { return errorIngestSecretPrefix + app }

// ErrorStore is the issue storage the error tracking routes need.
type ErrorStore interface {
	WriteErrorEvent(ctx context.Context, ev telemetry.ErrorEvent, keep int) (bool, error)
	ListErrorIssues(ctx context.Context, f telemetry.ErrorFilter) ([]telemetry.ErrorIssue, error)
	GetErrorIssue(ctx context.Context, resourceID, fingerprint string) (telemetry.ErrorIssue, []telemetry.ErrorSample, error)
	SetErrorIssueStatus(ctx context.Context, resourceID, fingerprint, status string) error
}

// WithErrorStore turns on the Sentry-compatible receiver and the error routes.
func WithErrorStore(s ErrorStore) Option {
	return func(rt *Router) { rt.errTrack.store = s }
}

type errorBucket struct {
	tokens float64
	last   time.Time
}

type deployIDEntry struct {
	id      string
	expires time.Time
}

// errorTrackingState is everything the receiver keeps in memory.
type errorTrackingState struct {
	store ErrorStore
	keys  traceKeyCache

	mu      sync.Mutex
	buckets map[string]*errorBucket
	deploys map[string]deployIDEntry
}

// allow takes one token from the app's bucket; when it is empty the second
// result is how long until one is back.
func (s *errorTrackingState) allow(app string, perMin int, now time.Time) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buckets == nil {
		s.buckets = map[string]*errorBucket{}
	}
	b := s.buckets[app]
	if b == nil {
		b = &errorBucket{tokens: float64(perMin), last: now}
		s.buckets[app] = b
	}
	rate := float64(perMin) / 60
	b.tokens = math.Min(float64(perMin), b.tokens+now.Sub(b.last).Seconds()*rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / rate * float64(time.Second))
	return false, wait
}

func envPositiveInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// currentDeployID is the app's newest deploy attempt, cached briefly so a
// burst of events costs one lookup.
func (rt *Router) currentDeployID(ctx context.Context, app string, now time.Time) string {
	s := &rt.errTrack
	s.mu.Lock()
	if e, ok := s.deploys[app]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return e.id
	}
	s.mu.Unlock()
	id := ""
	if attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, app); err == nil && len(attempts) > 0 {
		id = attempts[0].ID
	}
	s.mu.Lock()
	if s.deploys == nil {
		s.deploys = map[string]deployIDEntry{}
	}
	s.deploys[app] = deployIDEntry{id: id, expires: now.Add(deployIDCacheTTL)}
	s.mu.Unlock()
	return id
}

// sentryKey pulls the DSN public key from the places SDKs put it.
func sentryKey(r *http.Request) string {
	if h := r.Header.Get("X-Sentry-Auth"); h != "" {
		for _, part := range strings.Split(strings.TrimPrefix(h, "Sentry "), ",") {
			if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && k == "sentry_key" {
				return strings.Trim(v, `"`)
			}
		}
	}
	if k := r.URL.Query().Get("sentry_key"); k != "" {
		return k
	}
	if u, _, ok := r.BasicAuth(); ok {
		return u
	}
	return ""
}

func (rt *Router) authErrorIngest(r *http.Request, tok string) string {
	if tok == "" || rt.secrets == nil {
		return ""
	}
	app, ok := splitTraceToken(tok)
	if !ok {
		return ""
	}
	now := time.Now()
	want, cached := rt.errTrack.keys.get(app, now)
	if !cached {
		if !rt.ingestLookupAllowed(r) {
			return ""
		}
		v, err := rt.secrets.Resolve(r.Context(), errorIngestSecretsKey(app), errorIngestSecretName)
		if err != nil || v == "" {
			rt.ingestLookupFailed(r)
			return ""
		}
		want = v
		if subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
			rt.ingestLookupFailed(r)
			return ""
		}
		rt.errTrack.keys.put(app, want, now)
	}
	if subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
		return ""
	}
	return app
}

// ingestLookupAllowed reports whether this IP still has failed-guess budget for an
// uncached key lookup. It never spends: only failures do, so valid keys never throttle.
func (rt *Router) ingestLookupAllowed(r *http.Request) bool {
	return rt.buildHookLimits().failures.peek("ingest-probe:" + clientIP(r))
}

// ingestLookupFailed records one failed authentication against the client IP.
func (rt *Router) ingestLookupFailed(r *http.Request) {
	rt.buildHookLimits().failures.spend("ingest-probe:" + clientIP(r))
}

func setSentryCORS(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Headers", "Content-Type, X-Sentry-Auth, Authorization, Content-Encoding, Accept")
	h.Set("Access-Control-Expose-Headers", "Retry-After")
}

// handleSentryPreflight answers the browser SDK's CORS preflight.
func (rt *Router) handleSentryPreflight(w http.ResponseWriter, _ *http.Request) {
	setSentryCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

// handleSentryEnvelope handles POST /api/v1/sentry/api/{project}/envelope/ and
// the legacy .../store/. The DSN public key is the credential and names
// exactly one app.
func (rt *Router) handleSentryEnvelope(w http.ResponseWriter, r *http.Request) {
	rt.ingestSentry(w, r, false)
}

func (rt *Router) handleSentryStore(w http.ResponseWriter, r *http.Request) {
	rt.ingestSentry(w, r, true)
}

func (rt *Router) ingestSentry(w http.ResponseWriter, r *http.Request, legacy bool) {
	setSentryCORS(w)
	if rt.errTrack.store == nil {
		writeError(w, http.StatusNotImplemented, "error tracking is not configured on this control plane")
		return
	}
	app := rt.authErrorIngest(r, sentryKey(r))
	if app == "" {
		writeError(w, http.StatusUnauthorized, "missing or invalid DSN key")
		return
	}
	if ok, wait := rt.errTrack.allow(app, envPositiveInt(envErrorIngestPerMin, defaultErrorIngestRate), time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "error ingest rate limit exceeded")
		return
	}
	var src io.Reader = http.MaxBytesReader(w, r.Body, maxSentryBody)
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(src)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid gzip body")
			return
		}
		defer func() { _ = zr.Close() }()
		src = io.LimitReader(zr, maxSentryBody+1)
	}
	body, err := io.ReadAll(src)
	if err != nil || len(body) > maxSentryBody {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var events []telemetry.ErrorEvent
	if legacy {
		var ev telemetry.ErrorEvent
		if ev, err = erroringest.ParseStore(body); err == nil {
			events = []telemetry.ErrorEvent{ev}
		}
	} else {
		events, err = erroringest.ParseEnvelope(body)
	}
	if err != nil && len(events) == 0 {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now()
	deployID := rt.currentDeployID(r.Context(), app, now)
	keep := envPositiveInt(envErrorEventsPerIssue, telemetry.DefaultErrorEventsPerIssue)
	for _, ev := range events {
		ev.ResourceID = "service:" + app
		ev.DeployID = deployID
		if _, werr := rt.errTrack.store.WriteErrorEvent(r.Context(), ev, keep); werr != nil {
			rt.internalError(w, "api: write error event", werr, slog.String("app", app))
			return
		}
	}
	id := ""
	if len(events) > 0 {
		id = events[0].EventID
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}
