package apiclient

import (
	"context"
	"net/http"
	"time"
)

// IntegrityStatus is the result of the last control plane database check.
type IntegrityStatus struct {
	Healthy    bool      `json:"healthy"`
	Mode       string    `json:"mode"`
	CheckedAt  time.Time `json:"checked_at"`
	Error      string    `json:"error,omitempty"`
	Checks     int64     `json:"checks"`
	Failures   int64     `json:"failures"`
	LastFullAt time.Time `json:"last_full_at"`
}

// IntegrityPending is a snapshot staged to replace the database at the next boot.
type IntegrityPending struct {
	Name          string    `json:"name"`
	SHA256        string    `json:"sha256"`
	SchemaVersion int       `json:"schema_version"`
	StagedAt      time.Time `json:"staged_at"`
	StagedBy      string    `json:"staged_by,omitempty"`
}

// IntegrityCandidate is one snapshot that can be restored.
type IntegrityCandidate struct {
	Name       string     `json:"name"`
	SizeBytes  int64      `json:"size_bytes"`
	CreatedAt  time.Time  `json:"created_at"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	Verified   bool       `json:"verified"`
}

// Integrity is the /api/v1/system/integrity resource.
type Integrity struct {
	Status     IntegrityStatus      `json:"status"`
	Pending    *IntegrityPending    `json:"pending,omitempty"`
	Candidates []IntegrityCandidate `json:"candidates"`
}

// IntegrityRestore is the answer to staging a restore.
type IntegrityRestore struct {
	Pending IntegrityPending `json:"pending"`
	Restart struct {
		Supported bool   `json:"supported"`
		Hint      string `json:"hint,omitempty"`
	} `json:"restart"`
}

// GetIntegrity calls GET /api/v1/system/integrity.
func (c *Client) GetIntegrity(ctx context.Context) (Integrity, error) {
	var out Integrity
	err := c.do(ctx, http.MethodGet, "/api/v1/system/integrity", nil, &out)
	return out, err
}

// CheckIntegrity calls POST /api/v1/system/integrity/check.
func (c *Client) CheckIntegrity(ctx context.Context, full bool) (IntegrityStatus, error) {
	var out IntegrityStatus
	err := c.do(ctx, http.MethodPost, "/api/v1/system/integrity/check", map[string]bool{"full": full}, &out)
	return out, err
}

// RestoreIntegrity calls POST /api/v1/system/integrity/restore. An empty name picks the newest verified snapshot.
func (c *Client) RestoreIntegrity(ctx context.Context, name string, proof *StepUpProof) (IntegrityRestore, error) {
	var out IntegrityRestore
	body := struct {
		Name   string       `json:"name"`
		StepUp *StepUpProof `json:"step_up,omitempty"`
	}{name, proof}
	err := c.do(ctx, http.MethodPost, "/api/v1/system/integrity/restore", body, &out)
	return out, err
}

// CancelIntegrityRestore calls DELETE /api/v1/system/integrity/restore.
func (c *Client) CancelIntegrityRestore(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/system/integrity/restore", nil, nil)
}
