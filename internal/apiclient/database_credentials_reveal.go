package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DatabaseRevealedCredential mirrors internal/api's databaseRevealResponse.
type DatabaseRevealedCredential struct {
	Database string `json:"database"`
	Engine   string `json:"engine"`
	Password string `json:"password"`
}

// RevealDatabaseCredentials calls POST /api/v1/databases/{name}/credentials/reveal.
// It needs the token owner's step-up proof and is audited server side.
func (c *Client) RevealDatabaseCredentials(ctx context.Context, name string, proof *StepUpProof) (DatabaseRevealedCredential, error) {
	var out DatabaseRevealedCredential
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+url.PathEscape(name)+"/credentials/reveal", rotateDatabasePasswordRequest{StepUp: proof}, &out)
	return out, err
}
