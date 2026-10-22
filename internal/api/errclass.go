package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Stable machine codes for known causes; clients branch on these.
const (
	errCodeDockerUnavailable   = "docker_unavailable"
	errCodeTimeout             = "timeout"
	errCodeDiskFull            = "disk_full"
	errCodeDatabaseBusy        = "database_busy"
	errCodeRegistryUnreachable = "registry_unreachable"
	errCodeInternal            = "internal_error"

	envLeftoverVolumeTimeout = "APP_LEFTOVER_VOLUME_CHECK_TIMEOUT"
	defaultLeftoverTimeout   = 5 * time.Second

	dockerUnavailableMessage = "Docker is not responding on this server. Databases cannot be created until it is back."
)

type errorFix struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// typedError is the body of a classified failure. Error mirrors Message so
// clients that only read `error` still show the human text.
type typedError struct {
	Error     string    `json:"error"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Fix       *errorFix `json:"fix,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	status    int
}

// classifyError maps a wrapped cause to a typed response, or nil when the
// cause is unknown and should stay a generic 500.
func classifyError(err error) *typedError {
	if err == nil {
		return nil
	}
	mk := func(status int, code, msg string, fix *errorFix) *typedError {
		return &typedError{Error: msg, Code: code, Message: msg, Fix: fix, status: status}
	}
	text := strings.ToLower(err.Error())
	var netErr net.Error
	switch {
	case errors.Is(err, syscall.ENOSPC) || strings.Contains(text, "no space left on device"):
		return mk(http.StatusInsufficientStorage, errCodeDiskFull,
			"The server is out of disk space. Free some space and try again.",
			&errorFix{ID: "disk.reclaim", Label: "Free up disk space"})
	case errors.Is(err, docker.ErrDaemonUnresponsive) || dockerclient.IsErrConnectionFailed(err) ||
		strings.Contains(text, "docker-guard.sock") || strings.Contains(text, "cannot connect to the docker daemon"):
		return mk(http.StatusServiceUnavailable, errCodeDockerUnavailable, dockerUnavailableMessage, nil)
	case strings.Contains(text, "database is locked") || strings.Contains(text, "sqlite_busy") || strings.Contains(text, "sqlite_locked"):
		return mk(http.StatusServiceUnavailable, errCodeDatabaseBusy,
			"The control plane database is busy. Try again in a few seconds.", nil)
	case strings.Contains(text, "no such host") || strings.Contains(text, "registry") && (strings.Contains(text, "dial tcp") || strings.Contains(text, "timeout") || strings.Contains(text, "unreachable")):
		return mk(http.StatusBadGateway, errCodeRegistryUnreachable,
			"The image registry could not be reached. Check the server's network and try again.", nil)
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout()):
		return mk(http.StatusGatewayTimeout, errCodeTimeout,
			"The operation took too long and was stopped. Try again.", nil)
	}
	return nil
}

// leftoverVolumeTimeout bounds the leftover-volume Docker call so a hung
// daemon cannot hold a create request open.
func leftoverVolumeTimeout() time.Duration {
	if v, err := time.ParseDuration(os.Getenv(envLeftoverVolumeTimeout)); err == nil && v > 0 {
		return v
	}
	return defaultLeftoverTimeout
}

// dockerDegraded reports whether the Docker health state says the daemon is
// not answering, without making a Docker call.
func (rt *Router) dockerDegraded() bool {
	rep, ok := rt.dockerPinger.(unresponsiveReporter)
	return ok && !rep.UnresponsiveSince().IsZero()
}

// rejectIfDockerDegraded writes the typed 503 and returns true when Docker is down.
func (rt *Router) rejectIfDockerDegraded(w http.ResponseWriter) bool {
	if !rt.dockerDegraded() {
		return false
	}
	t := classifyError(docker.ErrDaemonUnresponsive)
	t.RequestID = w.Header().Get(requestIDHeader)
	writeJSON(w, t.status, t)
	return true
}
