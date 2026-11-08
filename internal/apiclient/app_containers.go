package apiclient

import (
	"context"
	"net/http"
	"time"
)

// AppContainerHealth mirrors internal/api's appContainerHealth.
type AppContainerHealth struct {
	Status        string `json:"status"`
	FailingStreak int    `json:"failing_streak"`
	LastOutput    string `json:"last_output"`
}

// AppContainerPort mirrors internal/api's appContainerPort.
type AppContainerPort struct {
	Container int    `json:"container"`
	Host      int    `json:"host"`
	Proto     string `json:"proto"`
	Bind      string `json:"bind"`
}

// AppContainerNetwork mirrors internal/api's appContainerNetwork.
type AppContainerNetwork struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip"`
	Aliases []string `json:"aliases"`
}

// AppContainerMount mirrors internal/api's appContainerMount.
type AppContainerMount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
	RO     bool   `json:"ro"`
}

// AppContainerLimits mirrors internal/api's appContainerLimits.
type AppContainerLimits struct {
	CPU         float64 `json:"cpu"`
	MemoryBytes int64   `json:"memory_bytes"`
}

// AppContainerStats mirrors internal/api's appContainerStats.
type AppContainerStats struct {
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryBytes      uint64  `json:"memory_bytes"`
	MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
}

// AppContainer mirrors internal/api's appContainerResource. Unavailable
// names fields the replica's node could not report.
type AppContainer struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Image        string                `json:"image"`
	ImageDigest  string                `json:"image_digest"`
	State        string                `json:"state"`
	RestartCount *int                  `json:"restart_count,omitempty"`
	StartedAt    *time.Time            `json:"started_at,omitempty"`
	Node         string                `json:"node"`
	Health       *AppContainerHealth   `json:"health,omitempty"`
	Ports        []AppContainerPort    `json:"ports"`
	Networks     []AppContainerNetwork `json:"networks"`
	Mounts       []AppContainerMount   `json:"mounts"`
	Limits       AppContainerLimits    `json:"limits"`
	Stats        *AppContainerStats    `json:"stats,omitempty"`
	Unavailable  []string              `json:"unavailable"`
}

// ListAppContainers calls GET /api/v1/apps/{name}/containers.
func (c *Client) ListAppContainers(ctx context.Context, name string) ([]AppContainer, error) {
	var out []AppContainer
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/containers", nil, &out)
	return out, err
}
