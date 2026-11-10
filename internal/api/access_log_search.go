package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AccessSearcher is the optional search surface of an AccessSource.
type AccessSearcher interface {
	AppKeys(app string) []string
	OwnedKeys() map[string]string
	SearchAccess(keys []string, q ingress.AccessSearch) ingress.AccessSearchResult
	PurgeApp(app string)
}

// accessSearchInflight counts running searches against the live concurrency cap.
var accessSearchInflight atomic.Int64

// AppKeys implements AccessSearcher.
func (e EdgeAccess) AppKeys(app string) []string { return e.Stats.KeysForApp(app) }

// OwnedKeys implements AccessSearcher.
func (e EdgeAccess) OwnedKeys() map[string]string { return e.Stats.OwnedKeys() }

// SearchAccess implements AccessSearcher.
func (e EdgeAccess) SearchAccess(keys []string, q ingress.AccessSearch) ingress.AccessSearchResult {
	return e.Log.Search(keys, q, e.Stats.OwnerSince())
}

// PurgeApp implements AccessSearcher: a deleted app's rows must not reach a
// later app that reuses its name before the next owner publication.
func (e EdgeAccess) PurgeApp(app string) {
	for _, key := range e.Stats.KeysForApp(app) {
		e.Log.Purge(key)
	}
}

func (rt *Router) purgeAppAccess(app string) {
	if s, ok := rt.access.(AccessSearcher); ok {
		s.PurgeApp(app)
	}
}

// accessSearchRow is one edge request. Client IP, user agent and referrer are
// never serialized by AccessRecord, and the query string is never recorded.
type accessSearchRow struct {
	ingress.AccessRecord
	App string `json:"app,omitempty"`
}

type accessSearchResponse struct {
	Records    []accessSearchRow `json:"records"`
	NextCursor string            `json:"next_cursor,omitempty"`
	Truncated  bool              `json:"truncated"`
	From       time.Time         `json:"from"`
	To         time.Time         `json:"to"`
	MaxRows    int               `json:"max_rows"`
	MaxWindow  int64             `json:"max_window_seconds"`
}

const defaultAccessSearchWindow = time.Hour

func (rt *Router) accessSearcher(w http.ResponseWriter) (AccessSearcher, bool) {
	s, ok := rt.access.(AccessSearcher)
	if !ok {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
	}
	return s, ok
}

// handleAppAccessSearch serves GET /api/v1/apps/{name}/access/search. The route
// wrapper answers 404 for apps hidden by team or scope and 403 for an explicit
// Deny (the house contract); an unknown app answers 404 here.
func (rt *Router) handleAppAccessSearch(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.accessSearcher(w)
	if !ok {
		return
	}
	app := r.PathValue("name")
	_, gerr := rt.apps.GetDesiredService(r.Context(), app)
	if gerr != nil && !errors.Is(gerr, store.ErrServiceNotFound) {
		rt.internalError(w, "api: access search: load app", gerr)
		return
	}
	if gerr != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	keys := s.AppKeys(app)
	rt.serveAccessSearch(w, r, s, keys, func(string) string { return app })
}

// handleAccessSearch serves GET /api/v1/logs/access across every app the
// caller can read. Route keys owned by no app are shown to instance admins only.
func (rt *Router) handleAccessSearch(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.accessSearcher(w)
	if !ok {
		return
	}
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: access search: caller scope", err)
		return
	}
	// A hidden or unknown app filter yields the same empty page, so the
	// response is never an oracle for which app names exist.
	want := r.URL.Query().Get("app")
	admin := rt.callerIsAdmin(r)
	owned := s.OwnedKeys()
	var keys []string
	for key, app := range owned {
		switch {
		case want != "" && app != want:
		case app == "" && !admin:
		case app != "" && !canRead(app):
		default:
			keys = append(keys, key)
		}
	}
	rt.serveAccessSearch(w, r, s, keys, func(key string) string { return owned[key] })
}

func (rt *Router) serveAccessSearch(w http.ResponseWriter, r *http.Request, s AccessSearcher, keys []string, appOf func(string) string) {
	limits := rt.RuntimeSettings().AccessSearchLimits(r.Context())
	q, err := parseAccessSearch(r, limits, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if accessSearchInflight.Add(1) > int64(limits.MaxConc) {
		accessSearchInflight.Add(-1)
		writeError(w, http.StatusTooManyRequests, "too many access log searches are running, retry shortly")
		return
	}
	res := s.SearchAccess(keys, q)
	accessSearchInflight.Add(-1)
	out := accessSearchResponse{
		Records: make([]accessSearchRow, len(res.Records)), Truncated: res.Truncated,
		From: q.From, To: q.To, MaxRows: limits.MaxRows, MaxWindow: int64(limits.MaxWindow / time.Second),
	}
	for i, rec := range res.Records {
		out.Records[i] = accessSearchRow{AccessRecord: rec, App: appOf(res.Keys[i])}
	}
	if res.Next != nil {
		out.NextCursor = encodeAccessCursor(*res.Next)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

// parseAccessSearch validates every filter against the live caps; an over-wide window is
// refused rather than clamped so the caller sees why rows are missing, while limit is clamped.
func parseAccessSearch(r *http.Request, lim runtimesettings.AccessSearchLimits, now time.Time) (ingress.AccessSearch, error) {
	v := r.URL.Query()
	q := ingress.AccessSearch{MaxScan: lim.MaxScan, To: now}
	var err error
	if t := v.Get("to"); t != "" {
		if q.To, err = time.Parse(time.RFC3339, t); err != nil {
			return q, errors.New("to must be RFC3339")
		}
	}
	switch {
	case v.Get("from") != "":
		if q.From, err = time.Parse(time.RFC3339, v.Get("from")); err != nil {
			return q, errors.New("from must be RFC3339")
		}
	case v.Get("since") != "":
		d, perr := time.ParseDuration(v.Get("since"))
		if perr != nil || d <= 0 {
			return q, errors.New("since must be a positive duration such as 15m")
		}
		q.From = q.To.Add(-d)
	default:
		q.From = q.To.Add(-min(defaultAccessSearchWindow, lim.MaxWindow))
	}
	if !q.From.Before(q.To) {
		return q, errors.New("from must be before to")
	}
	if q.To.Sub(q.From) > lim.MaxWindow {
		return q, fmt.Errorf("time window exceeds the %s maximum", lim.MaxWindow)
	}
	q.Limit = min(100, lim.MaxRows)
	if s := v.Get("limit"); s != "" {
		n, perr := strconv.Atoi(s)
		if perr != nil || n < 1 {
			return q, errors.New("limit must be a positive integer")
		}
		q.Limit = min(n, lim.MaxRows)
	}
	if q.StatusMin, q.StatusMax, err = parseStatusFilter(v.Get("status"), v.Get("status_min"), v.Get("status_max")); err != nil {
		return q, err
	}
	if s := v.Get("min_ms"); s != "" {
		if q.MinMillis, err = strconv.ParseFloat(s, 64); err != nil || q.MinMillis < 0 {
			return q, errors.New("min_ms must be a non-negative number")
		}
	}
	if q.Method = strings.ToUpper(v.Get("method")); q.Method != "" && !isHTTPMethodToken(q.Method) {
		return q, errors.New("method must be an HTTP method such as GET")
	}
	if q.Host = strings.TrimSpace(v.Get("host")); len(q.Host) > 253 {
		return q, errors.New("host is too long")
	}
	q.PathPrefix, q.Text = v.Get("path"), v.Get("q")
	if q.PathPrefix != "" && !strings.HasPrefix(q.PathPrefix, "/") {
		return q, errors.New("path must start with /")
	}
	if len(q.PathPrefix) > lim.MaxTerm || len(q.Text) > lim.MaxTerm {
		return q, fmt.Errorf("path and q may be at most %d bytes", lim.MaxTerm)
	}
	if c := v.Get("cursor"); c != "" {
		cur, cerr := decodeAccessCursor(c)
		if cerr != nil {
			return q, cerr
		}
		q.Before = &cur
	}
	return q, nil
}

// parseStatusFilter accepts an exact code ("404"), a class ("5xx") or explicit bounds.
func parseStatusFilter(status, lo, hi string) (int, int, error) {
	bad := errors.New("status must be a code such as 404 or a class such as 5xx")
	if status != "" {
		if len(status) == 3 && strings.HasSuffix(strings.ToLower(status), "xx") && status[0] >= '1' && status[0] <= '5' {
			c := int(status[0]-'0') * 100
			return c, c + 99, nil
		}
		n, err := strconv.Atoi(status)
		if err != nil || n < 100 || n > 599 {
			return 0, 0, bad
		}
		return n, n, nil
	}
	var a, b int
	var err error
	if lo != "" {
		if a, err = strconv.Atoi(lo); err != nil || a < 0 || a > 599 {
			return 0, 0, errors.New("status_min must be between 0 and 599")
		}
	}
	if hi != "" {
		if b, err = strconv.Atoi(hi); err != nil || b < 100 || b > 599 || b < a {
			return 0, 0, errors.New("status_max must be between 100 and 599 and not below status_min")
		}
	}
	return a, b, nil
}

func isHTTPMethodToken(m string) bool {
	if len(m) > 16 {
		return false
	}
	for _, c := range m {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func encodeAccessCursor(c ingress.AccessCursor) string {
	raw := strconv.FormatInt(c.Time.UnixNano(), 10) + ":" + c.RequestID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeAccessCursor(s string) (ingress.AccessCursor, error) {
	bad := errors.New("cursor is not valid")
	if len(s) > 256 {
		return ingress.AccessCursor{}, bad
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return ingress.AccessCursor{}, bad
	}
	ns, id, ok := strings.Cut(string(raw), ":")
	n, perr := strconv.ParseInt(ns, 10, 64)
	if !ok || perr != nil {
		return ingress.AccessCursor{}, bad
	}
	return ingress.AccessCursor{Time: time.Unix(0, n), RequestID: id}, nil
}
