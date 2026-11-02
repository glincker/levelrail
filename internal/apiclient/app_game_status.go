package apiclient

import (
	"context"
	"net/http"
)

// AppGameStatus is the response of GET /api/v1/apps/{name}/game-status.
type AppGameStatus struct {
	App         string `json:"app"`
	Exposure    string `json:"exposure"`
	Running     bool   `json:"running"`
	Online      bool   `json:"online"`
	Kind        string `json:"kind,omitempty"`
	Version     string `json:"version,omitempty"`
	Players     int    `json:"players"`
	MaxPlayers  int    `json:"max_players"`
	LatencyMS   int64  `json:"latency_ms"`
	Description string `json:"motd,omitempty"`
	JoinAddress string `json:"join_address,omitempty"`
	JoinPort    int    `json:"join_port,omitempty"`
	CheckedAt   string `json:"checked_at"`
}

// GetAppGameStatus calls GET /api/v1/apps/{name}/game-status.
func (c *Client) GetAppGameStatus(ctx context.Context, app string) (AppGameStatus, error) {
	var out AppGameStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/game-status", nil, &out)
	return out, err
}
