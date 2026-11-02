package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DoctorExplanation is the plain-language text for one doctor check.
type DoctorExplanation struct {
	ID    string `json:"id"`
	Plain string `json:"plain"`
	Why   string `json:"why"`
	Next  string `json:"next"`
	FixID string `json:"fix_id,omitempty"`
	Kind  string `json:"kind"`
}

// GetDoctorExplanation reads GET /api/v1/doctor/explain?id=<check-id>.
func (c *Client) GetDoctorExplanation(ctx context.Context, id string) (DoctorExplanation, error) {
	var out DoctorExplanation
	err := c.do(ctx, http.MethodGet, "/api/v1/doctor/explain?id="+url.QueryEscape(id), nil, &out)
	return out, err
}
