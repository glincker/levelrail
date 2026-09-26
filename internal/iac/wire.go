package iac

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Doer performs one JSON request against the control plane REST API. The
// CLI implements it over HTTP with the caller's token; the server
// implements it in process with the caller's own credentials, so per
// resource authorization applies to every call either way.
type Doer interface {
	Do(ctx context.Context, method, path string, body, out any) error
}

// StatusError is a non-2xx API answer.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Message) }

// IsDenied reports whether err is an authorization failure.
func IsDenied(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Status == http.StatusForbidden || se.Status == http.StatusUnauthorized)
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == http.StatusNotFound
}

func esc(s string) string { return url.PathEscape(s) }

type wireProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type wireEnvironment struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

type wireTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type wireDatabase struct {
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Version   string `json:"version"`
	ProjectID string `json:"project_id,omitempty"`
}

type wireApp struct {
	Name          string                    `json:"name"`
	Image         string                    `json:"image"`
	Port          int                       `json:"port"`
	HostPort      *int                      `json:"host_port,omitempty"`
	BindAddress   string                    `json:"bind_address"`
	Domains       []string                  `json:"domains,omitempty"`
	Env           map[string]string         `json:"env,omitempty"`
	SecretEnv     []string                  `json:"secret_env,omitempty"`
	VaultEnv      map[string]map[string]any `json:"vault_env,omitempty"`
	Resources     *store.ServiceResources   `json:"resources,omitempty"`
	Health        *store.ServiceHealth      `json:"health,omitempty"`
	Hooks         *store.ServiceHooks       `json:"hooks,omitempty"`
	Strategy      string                    `json:"strategy"`
	Replicas      int                       `json:"replicas"`
	Labels        map[string]string         `json:"labels,omitempty"`
	Command       []string                  `json:"command,omitempty"`
	ProjectID     string                    `json:"project_id,omitempty"`
	EnvironmentID string                    `json:"environment_id,omitempty"`
	Tags          []string                  `json:"tags,omitempty"`
	Volumes       []map[string]any          `json:"volumes,omitempty"`
	BindMounts    []map[string]any          `json:"bind_mounts,omitempty"`
	Egress        map[string]any            `json:"egress,omitempty"`
	EnvDirty      bool                      `json:"env_dirty"`
}

type wireSecretKey struct {
	Key string `json:"key"`
}

type wirePipeline struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Enabled bool   `json:"enabled"`
	YAML    string `json:"yaml"`
}

type wireAlert struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	Kind                  string  `json:"kind"`
	ChannelID             string  `json:"channel_id,omitempty"`
	Metric                string  `json:"metric,omitempty"`
	Comparator            string  `json:"comparator,omitempty"`
	Threshold             float64 `json:"threshold"`
	ForDuration           string  `json:"for_duration,omitempty"`
	RestartCountThreshold int     `json:"restart_count_threshold"`
	RestartWindow         string  `json:"restart_window,omitempty"`
	Enabled               bool    `json:"enabled"`
}

type wireChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type wireLB struct {
	Configured bool           `json:"configured"`
	Config     map[string]any `json:"config,omitempty"`
}
