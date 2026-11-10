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
	// Headers carry names and secret references only; values are read from
	// the app env at run time and never returned.
	Headers []jobHeaderResource `json:"headers,omitempty"`
}

type jobHeaderResource struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
}

const maxJobHTTPHeaders = 20

func validateJobHeaders(hs []jobHeaderResource) error {
	if len(hs) > maxJobHTTPHeaders {
		return fmt.Errorf("http.headers allows at most %d headers", maxJobHTTPHeaders)
	}
	for i, h := range hs {
		if strings.TrimSpace(h.Name) == "" || strings.ContainsAny(h.Name, " :\r\n") {
			return fmt.Errorf("http.headers[%d]: invalid header name", i)
		}
		if (h.Value == "") == (h.SecretRef == "") {
			return fmt.Errorf("http.headers[%d]: set exactly one of value or secretRef", i)
		}
		if strings.ContainsAny(h.Value+h.Prefix, "\r\n") {
			return fmt.Errorf("http.headers[%d]: value must be one line", i)
		}
	}
	return nil
}

func headersToStore(hs []jobHeaderResource) []store.ScheduledTaskHeader {
	out := make([]store.ScheduledTaskHeader, 0, len(hs))
	for _, h := range hs {
		out = append(out, store.ScheduledTaskHeader{Name: h.Name, Value: h.Value, SecretRef: h.SecretRef, Prefix: h.Prefix})
	}
	return out
}

func headersFromStore(hs []store.ScheduledTaskHeader) []jobHeaderResource {
	out := make([]jobHeaderResource, 0, len(hs))
	for _, h := range hs {
		out = append(out, jobHeaderResource{Name: h.Name, Value: h.Value, SecretRef: h.SecretRef, Prefix: h.Prefix})
	}
	return out
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
	return validateJobHeaders(h.Headers)
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
		Headers: headersToStore(h.Headers),
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
	res.HTTP = &jobHTTPResource{Method: h.Method, Path: h.Path, Body: h.Body, Background: h.Background, Headers: headersFromStore(h.Headers)}
	return res
}

// jobResource is toScheduledTaskResource plus the http request, if any.
func (rt *Router) jobResource(ctx context.Context, t store.ScheduledTask) scheduledTaskResource {
	return rt.withJobGuards(ctx, rt.withJobHeartbeat(ctx, rt.withJobHTTP(ctx, toScheduledTaskResource(t))))
}
