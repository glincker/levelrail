package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// IaCFile is one resource file sent to plan or apply.
type IaCFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// IaCRequest mirrors internal/api's iacRequest, the body of POST
// /api/v1/apply/plan and /api/v1/apply.
type IaCRequest struct {
	Files            []IaCFile         `json:"files"`
	Source           string            `json:"source,omitempty"`
	Project          string            `json:"project,omitempty"`
	Prune            bool              `json:"prune,omitempty"`
	Vars             map[string]string `json:"vars,omitempty"`
	Secrets          map[string]string `json:"secrets,omitempty"`
	NoDeploy         bool              `json:"no_deploy,omitempty"`
	ContinueOnError  bool              `json:"continue_on_error,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty"`
}

// IaCIssuesError carries the line numbered validation issues of a 422.
type IaCIssuesError struct{ Issues []iac.Issue }

func (e *IaCIssuesError) Error() string {
	lines := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		lines[i] = is.String()
	}
	return "invalid resource files:\n  " + strings.Join(lines, "\n  ")
}

func asIssues(err error) error {
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != http.StatusUnprocessableEntity {
		return err
	}
	var body struct {
		Issues []iac.Issue `json:"issues"`
	}
	if json.Unmarshal([]byte(api.Message), &body) != nil || len(body.Issues) == 0 {
		return err
	}
	return &IaCIssuesError{Issues: body.Issues}
}

// PlanIaC calls POST /api/v1/apply/plan.
func (c *Client) PlanIaC(ctx context.Context, req IaCRequest) (iac.Plan, error) {
	var out struct {
		Plan *iac.Plan `json:"plan"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/apply/plan", req, &out); err != nil {
		return iac.Plan{}, asIssues(err)
	}
	if out.Plan == nil {
		return iac.Plan{}, errors.New("plan response held no plan")
	}
	return *out.Plan, nil
}

// ApplyIaC calls POST /api/v1/apply.
func (c *Client) ApplyIaC(ctx context.Context, req IaCRequest) (iac.ApplyResult, error) {
	var out struct {
		Result *iac.ApplyResult `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/apply", req, &out); err != nil {
		return iac.ApplyResult{}, asIssues(err)
	}
	if out.Result == nil {
		return iac.ApplyResult{}, errors.New("apply response held no result")
	}
	return *out.Result, nil
}

// ExportIaC calls GET /api/v1/export.
func (c *Client) ExportIaC(ctx context.Context, project, app string, includeEnvValues bool) (iac.ExportResult, error) {
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	if app != "" {
		q.Set("app", app)
	}
	q.Set("include_env_values", fmt.Sprint(includeEnvValues))
	var out iac.ExportResult
	err := c.do(ctx, http.MethodGet, "/api/v1/export?"+q.Encode(), nil, &out)
	return out, err
}
