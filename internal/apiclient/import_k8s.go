package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// K8sImportOptions are the query parameters of POST /api/v1/import/k8s.
type K8sImportOptions struct {
	DryRun      bool
	Force       bool
	AppPrefix   string
	NamespaceAs string
	Project     string
}

// K8sFinding is one field the import could not map.
type K8sFinding struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Field    string `json:"field"`
	Reason   string `json:"reason"`
	Severity string `json:"severity"`
}

// K8sApp summarizes one app the import creates.
type K8sApp struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Image      string   `json:"image"`
	Port       int      `json:"port,omitempty"`
	Replicas   int      `json:"replicas"`
	Domains    []string `json:"domains,omitempty"`
	EnvKeys    []string `json:"envKeys,omitempty"`
	SecretKeys []string `json:"secretKeys,omitempty"`
}

// K8sImportResult mirrors internal/api's k8sImportResponse.
type K8sImportResult struct {
	DryRun    bool             `json:"dry_run"`
	Applied   bool             `json:"applied"`
	Blockers  int              `json:"blockers"`
	Documents string           `json:"documents"`
	Apps      []K8sApp         `json:"apps"`
	Findings  []K8sFinding     `json:"findings"`
	Result    *iac.ApplyResult `json:"result,omitempty"`
	Issues    []iac.Issue      `json:"issues,omitempty"`
}

// ImportK8s calls POST /api/v1/import/k8s with the manifest text.
func (c *Client) ImportK8s(ctx context.Context, manifest string, o K8sImportOptions) (K8sImportResult, error) {
	q := url.Values{}
	q.Set("dry_run", strconv.FormatBool(o.DryRun))
	q.Set("force", strconv.FormatBool(o.Force))
	for k, v := range map[string]string{"app_prefix": o.AppPrefix, "namespace_as": o.NamespaceAs, "project": o.Project} {
		if v != "" {
			q.Set(k, v)
		}
	}
	var out K8sImportResult
	body := struct {
		Manifest string `json:"manifest"`
	}{manifest}
	err := c.do(ctx, http.MethodPost, "/api/v1/import/k8s?"+q.Encode(), body, &out)
	var api *APIError
	if errors.As(err, &api) && api.StatusCode == http.StatusUnprocessableEntity {
		// A refused import still carries the report the caller needs to read.
		var refused K8sImportResult
		if json.Unmarshal([]byte(api.Message), &refused) == nil && (len(refused.Findings) > 0 || len(refused.Issues) > 0) {
			return refused, err
		}
	}
	return out, err
}
