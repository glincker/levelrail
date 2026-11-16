package gitlabapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// DeploymentStatus is GitLab's own "status" enum for a deployment.
type DeploymentStatus string

// Deployment statuses Levelrail reports.
const (
	DeploymentRunning  DeploymentStatus = "running"
	DeploymentSuccess  DeploymentStatus = "success"
	DeploymentFailed   DeploymentStatus = "failed"
	DeploymentCanceled DeploymentStatus = "canceled"
)

// Environment is a GitLab environment as the Environments API returns it.
type Environment struct {
	ID          int64
	Name        string
	ExternalURL string
}

type environmentResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ExternalURL string `json:"external_url"`
}

func (r environmentResponse) env() Environment { return Environment(r) }

type createEnvironmentRequest struct {
	Name        string `json:"name"`
	ExternalURL string `json:"external_url,omitempty"`
	Tier        string `json:"tier,omitempty"`
}

type updateEnvironmentRequest struct {
	ExternalURL string `json:"external_url"`
}

type createDeploymentRequest struct {
	Environment string `json:"environment"`
	SHA         string `json:"sha"`
	Ref         string `json:"ref"`
	Tag         bool   `json:"tag"`
	Status      string `json:"status"`
}

type updateDeploymentRequest struct {
	Status string `json:"status"`
}

func (c *Client) projectURL(instanceURL, projectPath, suffix string) string {
	return fmt.Sprintf("%s/projects/%s%s", apiBaseURL(instanceURL), encodedProjectID(projectPath), suffix)
}

// FindEnvironment returns the environment called name, or ok=false when the
// project has none.
func (c *Client) FindEnvironment(ctx context.Context, instanceURL, accessToken, projectPath, name string) (Environment, bool, error) {
	u := c.projectURL(instanceURL, projectPath, "/environments?name="+url.QueryEscape(name))
	var list []environmentResponse
	if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &list); err != nil {
		return Environment{}, false, err
	}
	for _, e := range list {
		if e.Name == name {
			return e.env(), true, nil
		}
	}
	return Environment{}, false, nil
}

// EnsureEnvironment returns the environment called name, creating it when it
// is missing. A repeat call is a no-op, and a lost create race falls back to
// the environment that won.
func (c *Client) EnsureEnvironment(ctx context.Context, instanceURL, accessToken, projectPath, name, externalURL, tier string) (Environment, error) {
	if e, ok, err := c.FindEnvironment(ctx, instanceURL, accessToken, projectPath, name); err != nil {
		return Environment{}, err
	} else if ok {
		return e, nil
	}
	payload, err := json.Marshal(createEnvironmentRequest{Name: name, ExternalURL: externalURL, Tier: tier})
	if err != nil {
		return Environment{}, fmt.Errorf("gitlabapp: marshal environment request: %w", err)
	}
	var out environmentResponse
	if err := c.do(ctx, http.MethodPost, c.projectURL(instanceURL, projectPath, "/environments"), "Bearer "+accessToken, bytes.NewReader(payload), &out); err != nil {
		if e, ok, ferr := c.FindEnvironment(ctx, instanceURL, accessToken, projectPath, name); ferr == nil && ok {
			return e, nil
		}
		return Environment{}, err
	}
	created := out.env()
	if created.ExternalURL == "" {
		created.ExternalURL = externalURL
	}
	return created, nil
}

// SetEnvironmentURL points an environment at the page it serves.
func (c *Client) SetEnvironmentURL(ctx context.Context, instanceURL, accessToken, projectPath string, envID int64, externalURL string) error {
	payload, err := json.Marshal(updateEnvironmentRequest{ExternalURL: externalURL})
	if err != nil {
		return fmt.Errorf("gitlabapp: marshal environment update: %w", err)
	}
	u := c.projectURL(instanceURL, projectPath, "/environments/"+strconv.FormatInt(envID, 10))
	return c.do(ctx, http.MethodPut, u, "Bearer "+accessToken, bytes.NewReader(payload), nil)
}

// StopEnvironment stops the environment called name. A missing environment is
// not an error: there is nothing left to stop.
func (c *Client) StopEnvironment(ctx context.Context, instanceURL, accessToken, projectPath, name string) error {
	e, ok, err := c.FindEnvironment(ctx, instanceURL, accessToken, projectPath, name)
	if err != nil || !ok {
		return err
	}
	u := c.projectURL(instanceURL, projectPath, "/environments/"+strconv.FormatInt(e.ID, 10)+"/stop")
	return c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, nil, nil)
}

// CreateDeployment records a deployment of sha to the named environment and
// returns its ID. ref is the branch or tag the commit was deployed from.
func (c *Client) CreateDeployment(ctx context.Context, instanceURL, accessToken, projectPath, environment, sha, ref string, tag bool, status DeploymentStatus) (int64, error) {
	payload, err := json.Marshal(createDeploymentRequest{Environment: environment, SHA: sha, Ref: ref, Tag: tag, Status: string(status)})
	if err != nil {
		return 0, fmt.Errorf("gitlabapp: marshal deployment request: %w", err)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, c.projectURL(instanceURL, projectPath, "/deployments"), "Bearer "+accessToken, bytes.NewReader(payload), &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// UpdateDeploymentStatus moves a deployment to status. Sending the same status
// twice is harmless.
func (c *Client) UpdateDeploymentStatus(ctx context.Context, instanceURL, accessToken, projectPath string, id int64, status DeploymentStatus) error {
	payload, err := json.Marshal(updateDeploymentRequest{Status: string(status)})
	if err != nil {
		return fmt.Errorf("gitlabapp: marshal deployment update: %w", err)
	}
	u := c.projectURL(instanceURL, projectPath, "/deployments/"+strconv.FormatInt(id, 10))
	return c.do(ctx, http.MethodPut, u, "Bearer "+accessToken, bytes.NewReader(payload), nil)
}
