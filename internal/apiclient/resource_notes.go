package apiclient

import (
	"context"
	"net/http"
)

// ResourceNotes is the description and markdown notes of an app or database.
type ResourceNotes struct {
	Description string `json:"description"`
	Notes       string `json:"notes"`
	UpdatedBy   string `json:"updated_by,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// ResourceNotesUpdate is the PUT body; a nil field keeps its stored value.
type ResourceNotesUpdate struct {
	Description *string `json:"description,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

// GetResourceNotes calls GET /api/v1/{kind}/{name}/notes, kind being apps or databases.
func (c *Client) GetResourceNotes(ctx context.Context, kind, name string) (ResourceNotes, error) {
	var out ResourceNotes
	err := c.do(ctx, http.MethodGet, "/api/v1/"+kind+"/"+PathEscape(name)+"/notes", nil, &out)
	return out, err
}

// SetResourceNotes calls PUT /api/v1/{kind}/{name}/notes.
func (c *Client) SetResourceNotes(ctx context.Context, kind, name string, u ResourceNotesUpdate) (ResourceNotes, error) {
	var out ResourceNotes
	err := c.do(ctx, http.MethodPut, "/api/v1/"+kind+"/"+PathEscape(name)+"/notes", u, &out)
	return out, err
}
