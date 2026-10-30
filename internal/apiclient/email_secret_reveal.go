package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

type revealEmailSecretRequest struct {
	StepUp *StepUpProof `json:"step_up,omitempty"`
}

// EmailSecretRevealed mirrors internal/api's emailSecretRevealResponse.
type EmailSecretRevealed struct {
	Value string `json:"value"`
}

// RevealEmailSecret calls POST /api/v1/settings/email/providers/{id}/secrets/{field}/reveal. It needs the token
// owner's step-up proof (password plus authenticator code) and is audited server side.
func (c *Client) RevealEmailSecret(ctx context.Context, id, field string, proof *StepUpProof) (EmailSecretRevealed, error) {
	var out EmailSecretRevealed
	path := "/api/v1/settings/email/providers/" + url.PathEscape(id) + "/secrets/" + url.PathEscape(field) + "/reveal"
	err := c.do(ctx, http.MethodPost, path, revealEmailSecretRequest{StepUp: proof}, &out)
	return out, err
}
