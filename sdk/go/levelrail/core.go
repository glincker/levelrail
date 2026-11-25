package levelrail

import (
	"context"
	"net/url"
	"strconv"
)

// nextCursorHeader carries the keyset cursor of the next list page.
const nextCursorHeader = "X-Next-Cursor"

// ListOptions narrows a list call. Zero values are omitted.
type ListOptions struct {
	Limit  int
	Cursor string
	Query  string
}

func (o ListOptions) values() url.Values {
	q := url.Values{}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Cursor != "" {
		q.Set("cursor", o.Cursor)
	}
	if o.Query != "" {
		q.Set("q", o.Query)
	}
	return q
}

// ListApps returns one page of apps and the cursor of the next page ("" at the end).
func (c *Client) ListApps(ctx context.Context, o ListOptions) ([]Object, string, error) {
	var out []Object
	h, err := c.Invoke(ctx, "handleListApps", nil, o.values(), nil, &out)
	if err != nil {
		return nil, "", err
	}
	return out, h.Get(nextCursorHeader), nil
}

// GetApp returns one app.
func (c *Client) GetApp(ctx context.Context, name string) (Object, error) {
	var out Object
	_, err := c.Invoke(ctx, "handleGetApp", []string{name}, nil, nil, &out)
	return out, err
}

// CreateApp creates an app from a JSON body (see the API reference for fields).
func (c *Client) CreateApp(ctx context.Context, body Object) (Object, error) {
	var out Object
	_, err := c.Invoke(ctx, "handleCreateApp", nil, nil, body, &out)
	return out, err
}

// DeployApp triggers a deploy. A nil body deploys the app as configured.
func (c *Client) DeployApp(ctx context.Context, name string, body Object) (Object, error) {
	var out Object
	_, err := c.Invoke(ctx, "handleTriggerDeploy", []string{name}, nil, body, &out)
	return out, err
}

// RollbackApp redeploys the release of an earlier deploy.
func (c *Client) RollbackApp(ctx context.Context, name, deployID string) (Object, error) {
	var out Object
	_, err := c.Invoke(ctx, "handleRollbackToDeploy", []string{name, deployID}, nil, Object{}, &out)
	return out, err
}

// ListDeploys returns the deploy history of an app.
func (c *Client) ListDeploys(ctx context.Context, name string) ([]Object, error) {
	var out []Object
	_, err := c.Invoke(ctx, "handleDeployHistory", []string{name}, nil, nil, &out)
	return out, err
}

// GetEnv returns the variables bound to an app. The server masks secret values.
func (c *Client) GetEnv(ctx context.Context, name string) ([]Object, error) {
	var out []Object
	_, err := c.Invoke(ctx, "handleListBoundEnv", []string{name}, nil, nil, &out)
	return out, err
}

// SetEnv replaces the plain environment of an app with vars and returns the
// updated app. Values are sent once and never logged or put into errors.
func (c *Client) SetEnv(ctx context.Context, name string, vars map[string]string) (Object, error) {
	var out Object
	_, err := c.Invoke(ctx, "handleUpdateApp", []string{name}, nil, Object{"env": vars}, &out)
	return out, err
}

// Health reports whether the control plane answers.
func (c *Client) Health(ctx context.Context) (Object, error) {
	var out Object
	_, err := c.Invoke(ctx, "handleHealthz", nil, nil, nil, &out)
	return out, err
}
