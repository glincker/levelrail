package apiclient

import (
	"context"
	"net/http"
	"time"
)

// EncryptionKeyFix mirrors internal/api's encryptionKeyFix.
type EncryptionKeyFix struct {
	Kind string `json:"kind"`
	App  string `json:"app,omitempty"`
}

// EncryptionKeyHint mirrors internal/api's encryptionKeyHint.
type EncryptionKeyHint struct {
	Code     string           `json:"code"`
	Severity string           `json:"severity"`
	Message  string           `json:"message"`
	Fix      EncryptionKeyFix `json:"fix"`
}

// AppKeyStatus mirrors internal/api's appKeyResource.
type AppKeyStatus struct {
	App        string     `json:"app"`
	CreatedAt  time.Time  `json:"created_at"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	AgeDays    int        `json:"age_days"`
	ValueCount int        `json:"value_count"`
	Overdue    bool       `json:"overdue"`
}

// MasterKeyStatus mirrors internal/api's masterKeyStatusResource.
type MasterKeyStatus struct {
	Configured bool       `json:"configured"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	AgeDays    *int       `json:"age_days,omitempty"`
	Overdue    bool       `json:"overdue"`
}

// KeyPolicy mirrors internal/api's keyPolicyResource.
type KeyPolicy struct {
	MaxKeyAgeDays   int        `json:"max_key_age_days"`
	RemindEveryDays int        `json:"remind_every_days"`
	LastRemindedAt  *time.Time `json:"last_reminded_at,omitempty"`
}

// KeyRotationRun mirrors internal/api's rotationRunResource.
type KeyRotationRun struct {
	ID         string     `json:"id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Failed     int        `json:"failed"`
	Actor      string     `json:"actor,omitempty"`
}

// EncryptionKeysStatus mirrors internal/api's encryptionKeysResource.
type EncryptionKeysStatus struct {
	MasterKey MasterKeyStatus     `json:"master_key"`
	Apps      []AppKeyStatus      `json:"apps"`
	Policy    KeyPolicy           `json:"policy"`
	LastRun   *KeyRotationRun     `json:"last_run,omitempty"`
	Hints     []EncryptionKeyHint `json:"hints"`
}

// RotateDEKFailure mirrors internal/api's rotateDEKFailure.
type RotateDEKFailure struct {
	App    string `json:"app"`
	Reason string `json:"reason"`
}

// RotateDEKResult mirrors internal/api's rotateDEKResponse.
type RotateDEKResult struct {
	App         string             `json:"app,omitempty"`
	Values      int                `json:"values,omitempty"`
	RotatedAt   *time.Time         `json:"rotated_at,omitempty"`
	RunID       string             `json:"run_id,omitempty"`
	Resumed     bool               `json:"resumed,omitempty"`
	Total       int                `json:"total,omitempty"`
	Rotated     int                `json:"rotated,omitempty"`
	Skipped     int                `json:"skipped,omitempty"`
	FailedCount int                `json:"failed_count,omitempty"`
	Failed      []RotateDEKFailure `json:"failed,omitempty"`
	Complete    bool               `json:"complete"`
}

type rotateDEKRequest struct {
	Service string       `json:"service,omitempty"`
	All     bool         `json:"all,omitempty"`
	StepUp  *StepUpProof `json:"step_up,omitempty"`
}

// KeyPolicyUpdate sets the maximum key age policy; nil fields stay unchanged.
type KeyPolicyUpdate struct {
	MaxKeyAgeDays   *int `json:"max_key_age_days,omitempty"`
	RemindEveryDays *int `json:"remind_every_days,omitempty"`
}

// GetEncryptionKeys calls GET /api/v1/system/encryption-keys.
func (c *Client) GetEncryptionKeys(ctx context.Context) (EncryptionKeysStatus, error) {
	var out EncryptionKeysStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/system/encryption-keys", nil, &out)
	return out, err
}

// RotateDEK calls POST /api/v1/system/encryption-keys/rotate for one app, or
// every app when app is empty. It needs the token owner's step-up proof.
func (c *Client) RotateDEK(ctx context.Context, app string, proof *StepUpProof) (RotateDEKResult, error) {
	var out RotateDEKResult
	err := c.do(ctx, http.MethodPost, "/api/v1/system/encryption-keys/rotate", rotateDEKRequest{Service: app, All: app == "", StepUp: proof}, &out)
	return out, err
}

// UpdateKeyPolicy calls PUT /api/v1/system/encryption-keys/policy.
func (c *Client) UpdateKeyPolicy(ctx context.Context, upd KeyPolicyUpdate) (EncryptionKeysStatus, error) {
	var out EncryptionKeysStatus
	err := c.do(ctx, http.MethodPut, "/api/v1/system/encryption-keys/policy", upd, &out)
	return out, err
}
