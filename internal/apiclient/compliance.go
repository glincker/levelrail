package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AccessReviewResource mirrors an access review campaign summary.
type AccessReviewResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	DueAt       string `json:"due_at"`
	Overdue     bool   `json:"overdue"`
	RepeatDays  int    `json:"repeat_days"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Total       int    `json:"total"`
	Decided     int    `json:"decided"`
	ToRevoke    int    `json:"to_revoke"`
}

// AccessReviewItem is one reviewable subject.
type AccessReviewItem struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	SubjectID    string `json:"subject_id"`
	Label        string `json:"label"`
	Role         string `json:"role"`
	Detail       string `json:"detail"`
	LastActiveAt string `json:"last_active_at"`
	Decision     string `json:"decision"`
	DecidedBy    string `json:"decided_by,omitempty"`
	AppliedAt    string `json:"applied_at,omitempty"`
	ApplyError   string `json:"apply_error,omitempty"`
}

// AccessReviewDetail is a campaign with its rows.
type AccessReviewDetail struct {
	AccessReviewResource
	Items []AccessReviewItem `json:"items"`
}

// CreateAccessReviewRequest is the POST body for a new campaign.
type CreateAccessReviewRequest struct {
	Name       string `json:"name,omitempty"`
	DueDays    int    `json:"due_days,omitempty"`
	RepeatDays int    `json:"repeat_days,omitempty"`
}

// CompleteAccessReviewResult is the answer of completing a campaign.
type CompleteAccessReviewResult struct {
	Revoked  int                `json:"revoked"`
	Failures []AccessReviewItem `json:"failures"`
}

// EvidencePackResource mirrors one evidence pack job.
type EvidencePackResource struct {
	ID         string `json:"id"`
	RangeFrom  string `json:"range_from"`
	RangeTo    string `json:"range_to"`
	Status     string `json:"status"`
	Step       string `json:"step"`
	Progress   int    `json:"progress"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// EvidenceFileCheck is one file result of a pack verification.
type EvidenceFileCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// EvidenceVerifyReport is the answer of POST .../verify.
type EvidenceVerifyReport struct {
	Valid            bool                `json:"valid"`
	SignatureChecked bool                `json:"signature_checked"`
	SignatureValid   bool                `json:"signature_valid"`
	KeyID            string              `json:"key_id,omitempty"`
	Files            []EvidenceFileCheck `json:"files"`
}

// ListAccessReviews calls GET /api/v1/compliance/access-reviews.
func (c *Client) ListAccessReviews(ctx context.Context) ([]AccessReviewResource, error) {
	var out []AccessReviewResource
	err := c.do(ctx, http.MethodGet, "/api/v1/compliance/access-reviews", nil, &out)
	return out, err
}

// CreateAccessReview calls POST /api/v1/compliance/access-reviews.
func (c *Client) CreateAccessReview(ctx context.Context, req CreateAccessReviewRequest) (AccessReviewResource, error) {
	var out AccessReviewResource
	err := c.do(ctx, http.MethodPost, "/api/v1/compliance/access-reviews", req, &out)
	return out, err
}

// GetAccessReview calls GET /api/v1/compliance/access-reviews/{id}.
func (c *Client) GetAccessReview(ctx context.Context, id string) (AccessReviewDetail, error) {
	var out AccessReviewDetail
	err := c.do(ctx, http.MethodGet, "/api/v1/compliance/access-reviews/"+PathEscape(id), nil, &out)
	return out, err
}

// DecideAccessReviewItem calls PUT .../access-reviews/{id}/items/{item}.
func (c *Client) DecideAccessReviewItem(ctx context.Context, id, item, decision string) error {
	path := "/api/v1/compliance/access-reviews/" + PathEscape(id) + "/items/" + PathEscape(item)
	return c.do(ctx, http.MethodPut, path, map[string]string{"decision": decision}, nil)
}

// CompleteAccessReview calls POST .../access-reviews/{id}/complete.
func (c *Client) CompleteAccessReview(ctx context.Context, id string) (CompleteAccessReviewResult, error) {
	var out CompleteAccessReviewResult
	err := c.do(ctx, http.MethodPost, "/api/v1/compliance/access-reviews/"+PathEscape(id)+"/complete", nil, &out)
	return out, err
}

// ListEvidencePacks calls GET /api/v1/compliance/evidence-packs.
func (c *Client) ListEvidencePacks(ctx context.Context) ([]EvidencePackResource, error) {
	var out []EvidencePackResource
	err := c.do(ctx, http.MethodGet, "/api/v1/compliance/evidence-packs", nil, &out)
	return out, err
}

// CreateEvidencePack calls POST /api/v1/compliance/evidence-packs.
func (c *Client) CreateEvidencePack(ctx context.Context, from, to string) (EvidencePackResource, error) {
	var out EvidencePackResource
	err := c.do(ctx, http.MethodPost, "/api/v1/compliance/evidence-packs", map[string]string{"from": from, "to": to}, &out)
	return out, err
}

// GetEvidencePack calls GET /api/v1/compliance/evidence-packs/{id}.
func (c *Client) GetEvidencePack(ctx context.Context, id string) (EvidencePackResource, error) {
	var out EvidencePackResource
	err := c.do(ctx, http.MethodGet, "/api/v1/compliance/evidence-packs/"+PathEscape(id), nil, &out)
	return out, err
}

// VerifyEvidencePack calls POST .../evidence-packs/{id}/verify.
func (c *Client) VerifyEvidencePack(ctx context.Context, id string) (EvidenceVerifyReport, error) {
	var out EvidenceVerifyReport
	err := c.do(ctx, http.MethodPost, "/api/v1/compliance/evidence-packs/"+PathEscape(id)+"/verify", nil, &out)
	return out, err
}

// DownloadEvidencePack streams the archive into w without buffering it.
func (c *Client) DownloadEvidencePack(ctx context.Context, id string, w io.Writer) error {
	url := c.baseURL + "/api/v1/compliance/evidence-packs/" + PathEscape(id) + "/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // baseURL is the operator-supplied API target
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	start := time.Now()
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		TraceRequest(http.MethodGet, url, "", time.Since(start), err)
		return fmt.Errorf("request GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	TraceRequest(http.MethodGet, url, resp.Status, time.Since(start), nil)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return &APIError{StatusCode: resp.StatusCode, Message: ExtractErrorMessage(data), RetryAfter: retryAfterHeader(resp.Header)}
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	return nil
}
