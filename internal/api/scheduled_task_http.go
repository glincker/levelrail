package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ScheduledTaskHTTPStore persists the request an http-mode job sends. It is
// optional on the scheduled task store: without it http jobs are refused.
type ScheduledTaskHTTPStore interface {
	SetScheduledTaskHTTP(ctx context.Context, h store.ScheduledTaskHTTP) error
	GetScheduledTaskHTTP(ctx context.Context, taskID string) (store.ScheduledTaskHTTP, error)
	DeleteScheduledTaskHTTP(ctx context.Context, taskID string) error
}

// jobHTTPResource is the wire shape of an http-mode job's request.
type jobHTTPResource struct {
	Method     string `json:"method,omitempty"`
	Path       string `json:"path"`
	Body       string `json:"body,omitempty"`
	Background bool   `json:"background,omitempty"`
}

const maxJobHTTPBody = 64 * 1024

var jobHTTPMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
}

// validateJobHTTP checks an http-mode job's request and defaults the method.
func validateJobHTTP(h *jobHTTPResource) error {
	if h == nil || strings.TrimSpace(h.Path) == "" {
		return errors.New("http.path is required for mode http")
	}
	if !strings.HasPrefix(h.Path, "/") || strings.HasPrefix(h.Path, "//") {
		return errors.New("http.path must start with a single /")
	}
	h.Method = strings.ToUpper(strings.TrimSpace(h.Method))
	if h.Method == "" {
		h.Method = http.MethodPost
	}
	if !jobHTTPMethods[h.Method] {
		return fmt.Errorf("http.method: %q is not one of GET, POST, PUT, PATCH, DELETE", h.Method)
	}
	if len(h.Body) > maxJobHTTPBody {
		return fmt.Errorf("http.body must be at most %d bytes", maxJobHTTPBody)
	}
	return nil
}

func (rt *Router) jobHTTPStore() ScheduledTaskHTTPStore {
	s, _ := rt.scheduledTasks.(ScheduledTaskHTTPStore)
	return s
}

// persistJobHTTP saves the request for an http job and clears it for any
// other mode, so a job switched away from http leaves nothing behind.
func (rt *Router) persistJobHTTP(ctx context.Context, task store.ScheduledTask, h *jobHTTPResource) error {
	hs := rt.jobHTTPStore()
	if hs == nil {
		if task.Mode == store.ScheduledTaskModeHTTP {
			return errors.New("http jobs are not supported by this store")
		}
		return nil
	}
	if task.Mode != store.ScheduledTaskModeHTTP {
		return hs.DeleteScheduledTaskHTTP(ctx, task.ID)
	}
	return hs.SetScheduledTaskHTTP(ctx, store.ScheduledTaskHTTP{
		TaskID: task.ID, Method: h.Method, Path: h.Path, Body: h.Body, Background: h.Background,
	})
}

// withJobHTTP attaches the stored request to an http-mode job's wire shape.
func (rt *Router) withJobHTTP(ctx context.Context, res scheduledTaskResource) scheduledTaskResource {
	hs := rt.jobHTTPStore()
	if hs == nil || res.Mode != store.ScheduledTaskModeHTTP {
		return res
	}
	h, err := hs.GetScheduledTaskHTTP(ctx, res.ID)
	if err != nil {
		if !errors.Is(err, store.ErrScheduledTaskHTTPNotFound) {
			rt.logger.Error("api: load job http target failed", slog.String("error", err.Error()), slog.String("id", res.ID))
		}
		return res
	}
	res.HTTP = &jobHTTPResource{Method: h.Method, Path: h.Path, Body: h.Body, Background: h.Background}
	return res
}

// jobResource is toScheduledTaskResource plus the http request, if any.
func (rt *Router) jobResource(ctx context.Context, t store.ScheduledTask) scheduledTaskResource {
	return rt.withJobHTTP(ctx, toScheduledTaskResource(t))
}
