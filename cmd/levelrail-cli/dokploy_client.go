package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// dokployApplication mirrors the subset of Dokploy's Application model
// (docs.dokploy.com/docs/api/reference-application) that field mapping
// reads. Verified against Dokploy's public API reference only, not a live
// instance: memoryLimit/cpuLimit are documented as raw bytes / nanoCPUs
// respectively (docs.dokploy.com/docs/core/applications/advanced) even
// though the update-endpoint schema types them as strings, and the exact
// nested shape returned by project.one for its application list could not
// be confirmed live, see dokployProjectDetail below.
type dokployApplication struct {
	ApplicationID string `json:"applicationId"`
	Name          string `json:"name"`
	SourceType    string `json:"sourceType"`
	BuildType     string `json:"buildType"`
	Dockerfile    string `json:"dockerfile"`
	DockerImage   string `json:"dockerImage"`

	Repository string `json:"repository"`
	Owner      string `json:"owner"`
	Branch     string `json:"branch"`

	GitlabRepository string `json:"gitlabRepository"`
	GitlabOwner      string `json:"gitlabOwner"`
	GitlabBranch     string `json:"gitlabBranch"`

	GiteaRepository string `json:"giteaRepository"`
	GiteaOwner      string `json:"giteaOwner"`
	GiteaBranch     string `json:"giteaBranch"`

	BitbucketRepository string `json:"bitbucketRepository"`
	BitbucketOwner      string `json:"bitbucketOwner"`
	BitbucketBranch     string `json:"bitbucketBranch"`

	CustomGitURL    string `json:"customGitUrl"`
	CustomGitBranch string `json:"customGitBranch"`

	// Env is a single newline-delimited KEY=VALUE blob, Dokploy's UI has
	// one textarea for env rather than Coolify's list of discrete
	// objects. Unlike Coolify, Dokploy's API has no documented
	// read:sensitive-style redaction: any caller with access to the app
	// gets real values back here.
	Env string `json:"env"`

	MemoryLimit string `json:"memoryLimit"`
	CPULimit    string `json:"cpuLimit"`
}

// dokployDomain mirrors Dokploy's Domain model
// (docs.dokploy.com/docs/api/reference-domain).
type dokployDomain struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// dokployProject mirrors one entry of GET /api/project.all.
type dokployProject struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}

// dokployApplicationSummary is the minimal per-application shape this
// client needs out of a project's nested application list.
type dokployApplicationSummary struct {
	ApplicationID string `json:"applicationId"`
	Name          string `json:"name"`
}

// dokployProjectDetail mirrors GET /api/project.one. Dokploy has no
// documented application.all (list-all) endpoint: applications live
// inside projects, so ListApplications walks project.all then
// project.one for each. The Applications field name and nesting here
// follow Dokploy's UI data model (a project owns its applications) but
// could not be verified against a live instance.
type dokployProjectDetail struct {
	ProjectID    string                      `json:"projectId"`
	Applications []dokployApplicationSummary `json:"applications"`
}

type dokployErrorBody struct {
	Message string `json:"message"`
	Error   struct {
		Message string `json:"message"`
	} `json:"error"`
}

// dokployAPIError is returned by DokployClient methods for a non-2xx
// response.
type dokployAPIError struct {
	StatusCode int
	Message    string
}

func (e *dokployAPIError) Error() string {
	return fmt.Sprintf("dokploy api returned %d: %s", e.StatusCode, e.Message)
}

// DokployClient is a minimal, read-only HTTP client for Dokploy's REST
// API. Dokploy's API is generated from its tRPC routers
// (docs.dokploy.com/api-reference/introduction): endpoints follow
// GET /api/<router>.<procedure>, authenticated with a single
// "x-api-key" header rather than a Bearer token. This migration tool
// never writes back to the source Dokploy instance.
type DokployClient struct {
	baseURL string
	apiKey  string
	hc      *http.Client
}

// NewDokployClient builds a DokployClient. baseURL is the Dokploy
// instance root, e.g. "https://dokploy.example.com"; "/api" is appended
// per request.
func NewDokployClient(baseURL, apiKey string) *DokployClient {
	return &DokployClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *DokployClient) do(ctx context.Context, path string, query url.Values, out any) error {
	u := c.baseURL + "/api" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	return providerGetJSON(ctx, c.hc, u, path, func(req *http.Request) {
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("Accept", "application/json")
	}, func(status int, data []byte) error {
		return &dokployAPIError{StatusCode: status, Message: extractDokployErrorMessage(data)}
	}, out)
}

func extractDokployErrorMessage(data []byte) string {
	var body dokployErrorBody
	if err := json.Unmarshal(data, &body); err == nil {
		if body.Error.Message != "" {
			return body.Error.Message
		}
		if body.Message != "" {
			return body.Message
		}
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// ListProjects calls GET /api/project.all.
func (c *DokployClient) ListProjects(ctx context.Context) ([]dokployProject, error) {
	var out []dokployProject
	err := c.do(ctx, "/project.all", nil, &out)
	return out, err
}

// GetProject calls GET /api/project.one?projectId={id}.
func (c *DokployClient) GetProject(ctx context.Context, projectID string) (dokployProjectDetail, error) {
	var out dokployProjectDetail
	err := c.do(ctx, "/project.one", url.Values{"projectId": {projectID}}, &out)
	return out, err
}

// ListApplications aggregates every application across every project,
// since Dokploy has no single list-all endpoint for applications.
func (c *DokployClient) ListApplications(ctx context.Context) ([]dokployApplicationSummary, error) {
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	var out []dokployApplicationSummary
	for _, p := range projects {
		detail, err := c.GetProject(ctx, p.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("get project %s: %w", p.ProjectID, err)
		}
		out = append(out, detail.Applications...)
	}
	return out, nil
}

// GetApplication calls GET /api/application.one?applicationId={id}.
func (c *DokployClient) GetApplication(ctx context.Context, applicationID string) (dokployApplication, error) {
	var out dokployApplication
	err := c.do(ctx, "/application.one", url.Values{"applicationId": {applicationID}}, &out)
	return out, err
}

// ListDomains calls GET /api/domain.byApplicationId?applicationId={id}.
func (c *DokployClient) ListDomains(ctx context.Context, applicationID string) ([]dokployDomain, error) {
	var out []dokployDomain
	err := c.do(ctx, "/domain.byApplicationId", url.Values{"applicationId": {applicationID}}, &out)
	return out, err
}
