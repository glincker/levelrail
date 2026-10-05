package apiclient

import (
	"context"
	"net/http"
	"time"
)

// AuthEngineMismatch mirrors internal/authengine's Mismatch.
type AuthEngineMismatch struct {
	At               time.Time `json:"at"`
	Kind             string    `json:"kind"`
	TokenID          string    `json:"token_id,omitempty"`
	LegacyOwnerID    string    `json:"legacy_owner_id,omitempty"`
	LibraryOwnerID   string    `json:"library_owner_id,omitempty"`
	LegacyAccepted   bool      `json:"legacy_accepted"`
	LibraryAccepted  bool      `json:"library_accepted"`
	LegacyAbilities  []string  `json:"legacy_abilities"`
	LibraryAbilities []string  `json:"library_abilities"`
}

// AuthEngineStatusResource mirrors internal/api's authEngineStatusResponse.
type AuthEngineStatusResource struct {
	Mode           string               `json:"mode"`
	LibraryVersion string               `json:"library_version"`
	Areas          []string             `json:"areas"`
	Compared       uint64               `json:"compared"`
	Matched        uint64               `json:"matched"`
	Mismatched     uint64               `json:"mismatched"`
	Dropped        uint64               `json:"dropped"`
	Skipped        uint64               `json:"skipped"`
	Errors         uint64               `json:"errors"`
	Mismatches     []AuthEngineMismatch `json:"mismatches"`
}

// GetAuthEngineStatus calls GET /api/v1/auth-engine/status (root only).
func (c *Client) GetAuthEngineStatus(ctx context.Context) (AuthEngineStatusResource, error) {
	var out AuthEngineStatusResource
	err := c.do(ctx, http.MethodGet, "/api/v1/auth-engine/status", nil, &out)
	return out, err
}
