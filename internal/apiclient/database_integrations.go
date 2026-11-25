package apiclient

import (
	"context"
	"net/http"
)

// DatabaseIntegrationState mirrors internal/dbintegrations.State.
type DatabaseIntegrationState struct {
	Code   string `json:"code"`
	Reason string `json:"reason,omitempty"`
}

// DatabaseIntegrationCard mirrors internal/dbintegrations.CardStatus.
type DatabaseIntegrationCard struct {
	ID          string                   `json:"id"`
	Category    string                   `json:"category"`
	Title       string                   `json:"title"`
	Summary     string                   `json:"summary"`
	Engines     []string                 `json:"engines"`
	Kind        string                   `json:"kind"`
	Docs        string                   `json:"docs"`
	DocsPlanned bool                     `json:"docs_planned"`
	Featured    bool                     `json:"featured"`
	State       DatabaseIntegrationState `json:"state"`
}

// DatabaseIntegrationsResource mirrors internal/api's databaseIntegrationsResource.
type DatabaseIntegrationsResource struct {
	Database     string                    `json:"database"`
	Engine       string                    `json:"engine"`
	Integrations []DatabaseIntegrationCard `json:"integrations"`
}

// ListDatabaseIntegrations calls GET /api/v1/databases/{name}/integrations.
func (c *Client) ListDatabaseIntegrations(ctx context.Context, name string) (DatabaseIntegrationsResource, error) {
	var out DatabaseIntegrationsResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/integrations", nil, &out)
	return out, err
}
