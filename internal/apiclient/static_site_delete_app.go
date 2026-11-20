package apiclient

import (
	"context"
	"errors"
	"net/http"
)

// DeleteStaticSiteAndApp deletes a site, and its backing app when the server
// answers 409 because the app still exists. Deleting the app removes the site.
func (c *Client) DeleteStaticSiteAndApp(ctx context.Context, name string) error {
	err := c.DeleteStaticSite(ctx, name)
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return c.DeleteApp(ctx, name)
	}
	return err
}
