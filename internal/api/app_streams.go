package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AppStreamStore is the store surface the app stream handlers need.
type AppStreamStore interface {
	SaveAppStream(ctx context.Context, s store.AppStream) error
	ListAppStreamsForService(ctx context.Context, serviceName string) ([]store.AppStream, error)
	ListAllAppStreams(ctx context.Context) ([]store.AppStream, error)
	DeleteAppStream(ctx context.Context, id string) error
	GetAppStream(ctx context.Context, id string) (store.AppStream, error)
}

// appStreamResource is the wire shape for a stored app stream.
type appStreamResource struct {
	ID            string `json:"id"`
	App           string `json:"app"`
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"`
	CreatedAt     string `json:"created_at"`
}

func toAppStreamResource(s store.AppStream) appStreamResource {
	return appStreamResource{
		ID:            s.ID,
		App:           s.ServiceName,
		ContainerPort: s.ContainerPort,
		HostPort:      s.HostPort,
		Protocol:      s.Protocol,
		CreatedAt:     s.CreatedAt,
	}
}

// createAppStreamRequest is handleCreateAppStream's request body.
type createAppStreamRequest struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol,omitempty"`
}

func validateCreateAppStreamRequest(req createAppStreamRequest) (store.AppStream, error) {
	if req.ContainerPort < 1 || req.ContainerPort > 65535 {
		return store.AppStream{}, errors.New("container_port must be between 1 and 65535")
	}
	if req.HostPort < 1 || req.HostPort > 65535 {
		return store.AppStream{}, errors.New("host_port must be between 1 and 65535")
	}
	protocol := req.Protocol
	if protocol == "" {
		protocol = store.AppStreamProtocolTCP
	}
	if protocol != store.AppStreamProtocolTCP {
		return store.AppStream{}, fmt.Errorf("protocol must be %q, udp is not supported yet", store.AppStreamProtocolTCP)
	}
	return store.AppStream{ContainerPort: req.ContainerPort, HostPort: req.HostPort, Protocol: protocol}, nil
}

// handleListAppStreams handles GET /api/v1/apps/{name}/streams.
func (rt *Router) handleListAppStreams(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.internalError(w, "api: list app streams: get app failed", err, slog.String("name", name))
		return
	}

	streams, err := rt.appStreams.ListAppStreamsForService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list app streams failed", err, slog.String("name", name))
		return
	}
	out := make([]appStreamResource, 0, len(streams))
	for _, s := range streams {
		out = append(out, toAppStreamResource(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateAppStream handles POST /api/v1/apps/{name}/streams.
// AbilityWriteSensitive, the same tier as a firewall rule. Bumps the
// owning service's RestartNonce on success so the next container
// recreation publishes the new port promptly (see
// internal/reconcile/application's streamPortBindings doc comment).
func (rt *Router) handleCreateAppStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.internalError(w, "api: create app stream: get app failed", err, slog.String("name", name))
		return
	}

	var req createAppStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	stream, err := validateCreateAppStreamRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stream.ServiceName = name

	existing, err := rt.appStreams.ListAllAppStreams(r.Context())
	if err != nil {
		rt.internalError(w, "api: create app stream: list existing failed", err, slog.String("name", name))
		return
	}
	for _, e := range existing {
		if e.HostPort == stream.HostPort {
			writeError(w, http.StatusConflict, fmt.Sprintf("host port %d is already used by a stream on app %q", stream.HostPort, e.ServiceName))
			return
		}
	}

	id, err := randomAppStreamID()
	if err != nil {
		rt.logger.Error("api: create app stream: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	stream.ID = id
	stream.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := rt.appStreams.SaveAppStream(r.Context(), stream); err != nil {
		rt.logger.Error("api: create app stream: save failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.RestartService(r.Context(), name); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		// The stream is saved; a restart-bump failure only delays when the
		// new port is actually published, picked up by this service's next
		// ordinary deploy or restart regardless. Logged, not fatal to this
		// request.
		rt.logger.Warn("api: create app stream: restart bump failed", slog.String("error", err.Error()), slog.String("name", name))
	}

	rt.recordAppEvent(r, store.AppEvent{
		AppName: name, Kind: store.AppEventConfigChange, Keys: []string{"streams"},
		Title:  "Config changed: streams",
		Detail: fmt.Sprintf("+ %d/%s -> container %d", stream.HostPort, stream.Protocol, stream.ContainerPort),
	})
	rt.nudgeReconciler()
	writeJSON(w, http.StatusCreated, toAppStreamResource(stream))
}

// handleDeleteAppStream handles DELETE /api/v1/apps/{name}/streams/{id}.
// AbilityWriteSensitive, same tier as create. Restart-bumps the owning
// service on success, the same reasoning handleCreateAppStream's own doc
// comment gives.
func (rt *Router) handleDeleteAppStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	stream, err := rt.appStreams.GetAppStream(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAppStreamNotFound) {
			writeError(w, http.StatusNotFound, "app stream not found")
			return
		}
		rt.internalError(w, "api: delete app stream: get failed", err, slog.String("id", id))
		return
	}
	if stream.ServiceName != name {
		writeError(w, http.StatusNotFound, "app stream not found")
		return
	}

	if err := rt.appStreams.DeleteAppStream(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrAppStreamNotFound) {
			writeError(w, http.StatusNotFound, "app stream not found")
			return
		}
		rt.internalError(w, "api: delete app stream failed", err, slog.String("id", id))
		return
	}

	if err := rt.apps.RestartService(r.Context(), name); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Warn("api: delete app stream: restart bump failed", slog.String("error", err.Error()), slog.String("name", name))
	}

	rt.recordAppEvent(r, store.AppEvent{
		AppName: name, Kind: store.AppEventConfigChange, Keys: []string{"streams"},
		Title:  "Config changed: streams",
		Detail: fmt.Sprintf("- %d/%s -> container %d", stream.HostPort, stream.Protocol, stream.ContainerPort),
	})
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// randomAppStreamID mirrors randomFirewallRuleID's exact shape.
func randomAppStreamID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate app stream id: %w", err)
	}
	return "stream_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
