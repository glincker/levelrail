package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	timelineDefaultLimit = 50
	timelineMaxLimit     = 200
	timelineDeployPrefix = "dpl_"
	timelineErrorLimit   = 200
)

// Timeline item statuses.
const (
	timelineSucceeded  = "succeeded"
	timelineFailed     = "failed"
	timelineInProgress = "in_progress"
	timelinePending    = "pending"
	timelineInfo       = "info"
)

type timelineRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type timelineItem struct {
	ID     string       `json:"id"`
	At     time.Time    `json:"at"`
	Kind   string       `json:"kind"`
	Status string       `json:"status"`
	Actor  string       `json:"actor"`
	Title  string       `json:"title"`
	Detail string       `json:"detail,omitempty"`
	Ref    *timelineRef `json:"ref,omitempty"`
}

type timelineResponse struct {
	Items      []timelineItem `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func encodeTimelineCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixNano(), 10) + "|" + id))
}

func decodeTimelineCursor(raw string) (*store.AppEventCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("invalid cursor")
	}
	ns, id, ok := strings.Cut(string(b), "|")
	n, perr := strconv.ParseInt(ns, 10, 64)
	if !ok || perr != nil || id == "" {
		return nil, errors.New("invalid cursor")
	}
	return &store.AppEventCursor{At: time.Unix(0, n).UTC(), ID: id}, nil
}

// timelineBefore reports whether an item sits strictly after the cursor in
// (time desc, id desc) order, i.e. is older.
func timelineBefore(at time.Time, id string, c *store.AppEventCursor) bool {
	if c == nil {
		return true
	}
	if !at.Equal(c.At) {
		return at.Before(c.At)
	}
	return id < c.ID
}

// handleAppTimeline handles GET /api/v1/apps/{name}/timeline: config and
// lifecycle events merged with deploy attempts, newest first.
func (rt *Router) handleAppTimeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: timeline: load app failed", err, slog.String("name", name))
		return
	}

	limit := timelineDefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, timelineMaxLimit)
	}
	var cursor *store.AppEventCursor
	if v := r.URL.Query().Get("before"); v != "" {
		c, err := decodeTimelineCursor(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cursor = c
	}

	var items []timelineItem
	if events := rt.appEvents(); events != nil {
		list, err := events.ListAppEvents(r.Context(), name, cursor, time.Time{}, nil, limit+1)
		if err != nil {
			rt.internalError(w, "api: timeline: list events failed", err, slog.String("name", name))
			return
		}
		for _, e := range list {
			items = append(items, eventTimelineItem(e))
		}
	}
	if rt.deployAttempts != nil {
		attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), name)
		if err != nil {
			rt.internalError(w, "api: timeline: list deploy attempts failed", err, slog.String("name", name))
			return
		}
		for _, it := range attemptTimelineItems(attempts) {
			if timelineBefore(it.At, it.ID, cursor) {
				items = append(items, it)
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].At.Equal(items[j].At) {
			return items[i].At.After(items[j].At)
		}
		return items[i].ID > items[j].ID
	})
	resp := timelineResponse{Items: []timelineItem{}}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		resp.NextCursor = encodeTimelineCursor(last.At, last.ID)
	}
	resp.Items = append(resp.Items, items...)
	writeJSON(w, http.StatusOK, resp)
}

func eventTimelineItem(e store.AppEvent) timelineItem {
	return timelineItem{
		ID: e.ID, At: e.CreatedAt.UTC(), Kind: e.Kind, Status: timelineInfo,
		Actor: e.Actor, Title: e.Title, Detail: e.Detail,
	}
}

// attemptTimelineItems turns deploy attempts (newest first) into timeline
// items. An attempt redeploying an image seen before its predecessor's is a
// rollback.
func attemptTimelineItems(attempts []store.DeployAttempt) []timelineItem {
	items := make([]timelineItem, 0, len(attempts))
	for i, a := range attempts {
		var older []store.DeployAttempt
		if i+1 < len(attempts) {
			older = attempts[i+1:]
		}
		items = append(items, attemptTimelineItem(a, isRollbackAttempt(a, older)))
	}
	return items
}

// isRollbackAttempt reports whether a deployed an image that an attempt older
// than its immediate predecessor had already deployed. older is newest first.
func isRollbackAttempt(a store.DeployAttempt, older []store.DeployAttempt) bool {
	if a.Source == store.DeployAttemptSourceAutoRollback || strings.Contains(a.Reason, "RollbackTo:") {
		return true
	}
	if len(older) < 2 || older[0].Image == a.Image {
		return false
	}
	for _, o := range older[1:] {
		if o.Status == store.DeployAttemptStatusSucceeded && o.Image == a.Image {
			return true
		}
	}
	return false
}

func attemptTimelineItem(a store.DeployAttempt, rollback bool) timelineItem {
	kind, verb := "deploy", "Deploy"
	if rollback {
		kind, verb = "rollback", "Rollback"
	}
	image := displayImage(a.Image)
	item := timelineItem{
		ID: timelineDeployPrefix + a.ID, At: a.StartedAt.UTC(), Kind: kind,
		Actor:  attemptActor(a.Source),
		Detail: attemptDetail(a),
		Ref:    &timelineRef{Type: "deploy_attempt", ID: a.ID},
	}
	switch a.Status {
	case store.DeployAttemptStatusSucceeded:
		item.Status, item.Title = timelineSucceeded, fmt.Sprintf("%s to %s", verb, image)
	case store.DeployAttemptStatusFailed:
		item.Status, item.Title = timelineFailed, fmt.Sprintf("%s to %s failed", verb, image)
	case store.DeployAttemptStatusRunning:
		item.Status, item.Title = timelineInProgress, fmt.Sprintf("%s to %s in progress", verb, image)
	case store.DeployAttemptStatusHeld:
		item.Status, item.Title = timelinePending, fmt.Sprintf("%s to %s held", verb, image)
	default:
		item.Status, item.Title = timelineInfo, fmt.Sprintf("%s to %s %s", verb, image, a.Status)
	}
	return item
}

func attemptActor(source string) string {
	switch source {
	case store.DeployAttemptSourceWebhook:
		return "webhook"
	case store.DeployAttemptSourceAutoRollback:
		return "auto-rollback"
	case store.DeployAttemptSourceManual, store.DeployAttemptSourceImage, "":
		return "manual"
	default:
		return source
	}
}

// displayImage shortens a digest-pinned reference to name@sha256:12hex.
func displayImage(image string) string {
	name, digest, ok := strings.Cut(image, "@")
	if !ok {
		return image
	}
	hex := strings.TrimPrefix(digest, "sha256:")
	return name + "@sha256:" + hex[:min(len(hex), 12)]
}

func attemptDetail(a store.DeployAttempt) string {
	var parts []string
	if a.CommitSHA != "" {
		parts = append(parts, "commit "+a.CommitSHA[:min(len(a.CommitSHA), 7)])
	}
	if a.ImageDigest != "" {
		parts = append(parts, "digest "+strings.TrimPrefix(a.ImageDigest, "sha256:")[:min(len(strings.TrimPrefix(a.ImageDigest, "sha256:")), 12)])
	}
	if a.Reason != "" {
		parts = append(parts, a.Reason)
	}
	if a.Error != "" {
		msg := a.Error
		if len(msg) > timelineErrorLimit {
			msg = msg[:timelineErrorLimit] + "..."
		}
		parts = append(parts, msg)
	}
	return strings.Join(parts, "; ")
}
