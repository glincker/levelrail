package apiclient

import (
	"context"
	"net/http"
)

// UpgradeHistoryItem is one recorded control plane version transition.
type UpgradeHistoryItem struct {
	ID                string  `json:"id"`
	Kind              string  `json:"kind"`
	FromVersion       string  `json:"from_version"`
	ToVersion         string  `json:"to_version"`
	Channel           string  `json:"channel"`
	SchemaBefore      *int    `json:"schema_before"`
	SchemaAfter       *int    `json:"schema_after"`
	SchemaMoved       bool    `json:"schema_moved"`
	OccurredAt        string  `json:"occurred_at"`
	Initiator         string  `json:"initiator"`
	Method            string  `json:"method"`
	BackupName        string  `json:"backup_name"`
	Health            string  `json:"health"`
	Notes             string  `json:"notes"`
	NotesState        string  `json:"notes_state"`
	ReleaseURL        string  `json:"release_url"`
	CompareURL        string  `json:"compare_url"`
	Acknowledged      bool    `json:"acknowledged"`
	AckedBy           string  `json:"acked_by"`
	AckedAt           *string `json:"acked_at"`
	RollbackAvailable bool    `json:"rollback_available"`
}

// AgentVersionChange is one observed node agent version change.
type AgentVersionChange struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	ObservedAt  string `json:"observed_at"`
}

// UpgradeHistory is GET /api/v1/updates/history's response.
type UpgradeHistory struct {
	CurrentVersion string               `json:"current_version"`
	Unacknowledged int                  `json:"unacknowledged"`
	Entries        []UpgradeHistoryItem `json:"entries"`
	AgentChanges   []AgentVersionChange `json:"agent_changes"`
}

// GetUpgradeHistory calls GET /api/v1/updates/history.
func (c *Client) GetUpgradeHistory(ctx context.Context) (UpgradeHistory, error) {
	var out UpgradeHistory
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/history", nil, &out)
	return out, err
}

// AckUpgrade calls POST /api/v1/updates/history/{id}/ack.
func (c *Client) AckUpgrade(ctx context.Context, id string) (UpgradeHistoryItem, error) {
	var out UpgradeHistoryItem
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/history/"+PathEscape(id)+"/ack", nil, &out)
	return out, err
}
