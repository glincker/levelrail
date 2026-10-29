package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Env names and defaults for v2 watch streams.
const (
	envWatchBuffer           = "APP_WATCH_BUFFER"
	envWatchMaxStreams       = "APP_WATCH_MAX_STREAMS"
	envWatchBookmarkInterval = "APP_WATCH_BOOKMARK_INTERVAL"
	envWatchPollInterval     = "APP_WATCH_POLL_INTERVAL"
	envWatchWriteTimeout     = "APP_WATCH_WRITE_TIMEOUT"

	v2EventBookmark = "BOOKMARK"
	v2EventError    = "ERROR"
	v2ReasonExpired = "Expired"
	v2ReasonSlow    = "TooSlow"
)

type v2WatchConfig struct {
	Buffer           int
	MaxStreams       int
	BookmarkInterval time.Duration
	PollInterval     time.Duration
	WriteTimeout     time.Duration
}

func v2WatchConfigFromEnv() v2WatchConfig {
	c := v2WatchConfig{Buffer: 256, MaxStreams: 64, BookmarkInterval: 30 * time.Second, PollInterval: time.Second, WriteTimeout: 10 * time.Second}
	if n, err := strconv.Atoi(os.Getenv(envWatchBuffer)); err == nil && n > 0 {
		c.Buffer = n
	}
	if n, err := strconv.Atoi(os.Getenv(envWatchMaxStreams)); err == nil && n > 0 {
		c.MaxStreams = n
	}
	for env, dst := range map[string]*time.Duration{
		envWatchBookmarkInterval: &c.BookmarkInterval,
		envWatchPollInterval:     &c.PollInterval,
		envWatchWriteTimeout:     &c.WriteTimeout,
	} {
		if d, err := time.ParseDuration(os.Getenv(env)); err == nil && d > 0 {
			*dst = d
		}
	}
	return c
}

func isV2Watch(r *http.Request) bool {
	v := r.URL.Query().Get("watch")
	return v == "true" || v == "1"
}

type v2Event struct {
	Type   string `json:"type"`
	Object any    `json:"object"`
}

type v2Status struct {
	Code            int    `json:"code"`
	Reason          string `json:"reason"`
	Message         string `json:"message"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

type v2Stream struct {
	w  http.ResponseWriter
	rc *http.ResponseController
	to time.Duration
}

func (st v2Stream) send(ev v2Event, id int64) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal watch event: %w", err)
	}
	_ = st.rc.SetWriteDeadline(time.Now().Add(st.to))
	if id > 0 {
		_, err = fmt.Fprintf(st.w, "event: %s\nid: %d\ndata: %s\n\n", ev.Type, id, data)
	} else {
		_, err = fmt.Fprintf(st.w, "event: %s\ndata: %s\n\n", ev.Type, data)
	}
	if err != nil {
		return fmt.Errorf("write watch event: %w", err)
	}
	if err := st.rc.Flush(); err != nil {
		return fmt.Errorf("flush watch event: %w", err)
	}
	return nil
}

func v2ExpiredMessage(from int64) string {
	return fmt.Sprintf("resourceVersion %d is older than the retained change history; list again and watch from the list's metadata.resourceVersion", from)
}

// handleWatch streams changes after resourceVersion over SSE. Events are
// filtered per object with the caller's v1 read check, and a consumer that
// falls more than one buffer behind is dropped instead of slowing the log.
func (s *v2Server) handleWatch(kind string, w http.ResponseWriter, r *http.Request) {
	if kind != v2PathApps {
		writeError(w, http.StatusBadRequest, "watch is supported for apps only; list "+kind+" instead")
		return
	}
	sel, err := parseV2Selector(r.URL.Query().Get("labelSelector"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	s.ensureBackfill(ctx)
	from, err := s.changes.CurrentResourceVersion(ctx)
	if err != nil {
		s.rt.internalError(w, "api: v2 watch: current version", err)
		return
	}
	if raw := r.URL.Query().Get("resourceVersion"); raw != "" {
		if from, err = strconv.ParseInt(raw, 10, 64); err != nil || from < 0 {
			writeError(w, http.StatusBadRequest, "resourceVersion must be a non-negative integer")
			return
		}
	}
	if _, err := s.changes.ResourceChangesSince(ctx, store.ResourceKindApp, from, 1); errors.Is(err, store.ErrResourceVersionCompacted) {
		writeJSON(w, http.StatusGone, v2Status{Code: http.StatusGone, Reason: v2ReasonExpired, Message: v2ExpiredMessage(from)})
		return
	} else if err != nil {
		s.rt.internalError(w, "api: v2 watch: probe", err)
		return
	}
	select {
	case s.streams <- struct{}{}:
		defer func() { <-s.streams }()
	default:
		writeError(w, http.StatusTooManyRequests, "too many watch streams open; retry later")
		return
	}
	canRead, _, err := s.rt.callerAppVisibility(r)
	if err != nil {
		s.rt.internalError(w, "api: v2 watch: resolve caller", err)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := s.rt.watchAuthorization(r, AbilityRead, func(*http.Request) string { return v2PathApps }, cancel)
	defer stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	st := v2Stream{w: w, rc: http.NewResponseController(w), to: s.cfg.WriteTimeout}
	if err := st.rc.Flush(); err != nil {
		return
	}
	s.streamApps(ctx, st, from, sel, canRead)
}

func (s *v2Server) streamApps(ctx context.Context, st v2Stream, cursor int64, sel v2Selector, canRead func(string) bool) {
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()
	bookmark := time.NewTicker(s.cfg.BookmarkInterval)
	defer bookmark.Stop()
	fullPolls := 0
	for {
		head, err := s.changes.CurrentResourceVersion(ctx)
		if err != nil {
			return
		}
		changes, err := s.changes.ResourceChangesSince(ctx, store.ResourceKindApp, cursor, s.cfg.Buffer)
		if errors.Is(err, store.ErrResourceVersionCompacted) {
			_ = st.send(v2Event{Type: v2EventError, Object: v2Status{Code: http.StatusGone, Reason: v2ReasonExpired, Message: v2ExpiredMessage(cursor)}}, 0)
			return
		}
		if err != nil {
			return
		}
		if len(changes) >= s.cfg.Buffer {
			fullPolls++
		} else {
			fullPolls = 0
		}
		if fullPolls > 1 {
			_ = st.send(v2Event{Type: v2EventError, Object: v2Status{Code: http.StatusGone, Reason: v2ReasonSlow, Message: "watch fell too far behind; list again and resume from the list's metadata.resourceVersion", ResourceVersion: v2Version(cursor)}}, 0)
			return
		}
		for _, c := range changes {
			cursor = c.Version
			ev, ok := s.appEvent(ctx, c, sel, canRead)
			if !ok {
				continue
			}
			if err := st.send(ev, c.Version); err != nil {
				s.rt.logger.Debug("api: v2 watch consumer dropped", slog.String("error", err.Error()))
				return
			}
		}
		if len(changes) < s.cfg.Buffer && head > cursor {
			cursor = head
		}
		select {
		case <-ctx.Done():
			return
		case <-bookmark.C:
			meta := v2Object{APIVersion: s.apiVersion(), Kind: v2KindApp, Metadata: v2Metadata{ResourceVersion: v2Version(cursor)}}
			if err := st.send(v2Event{Type: v2EventBookmark, Object: meta}, cursor); err != nil {
				return
			}
		case <-poll.C:
		}
	}
}

// appEvent maps one change for this caller; ok is false when the caller may
// not read the app or it does not match the selector.
func (s *v2Server) appEvent(ctx context.Context, c store.ResourceChange, sel v2Selector, canRead func(string) bool) (v2Event, bool) {
	if !canRead(c.Name) {
		return v2Event{}, false
	}
	svc, err := store.AppSnapshot(c.Object)
	if err != nil || svc.Name == "" {
		svc = store.DesiredService{Name: c.Name, ProjectID: c.Namespace}
	}
	if !sel.matches(svc.Labels) {
		return v2Event{}, false
	}
	objs, err := s.appObjects(ctx, []store.DesiredService{svc}, map[string]int64{svc.Name: c.Version}, false)
	if err != nil || len(objs) == 0 {
		return v2Event{}, false
	}
	if c.Op == store.ResourceOpDeleted {
		objs[0].Status = nil
	}
	return v2Event{Type: c.Op, Object: objs[0]}, true
}
