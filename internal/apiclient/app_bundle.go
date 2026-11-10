package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppBundleService is one service an imported bundle creates.
type AppBundleService struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

// AppBundleImport is the plan of a dry run or the result of an import.
// SecretsToSet lists names only; a bundle never carries secret values.
type AppBundleImport struct {
	App          string             `json:"app"`
	DryRun       bool               `json:"dry_run"`
	SHA256       string             `json:"sha256"`
	Services     []AppBundleService `json:"services"`
	SecretsToSet []string           `json:"secrets_to_set,omitempty"`
	Warnings     []string           `json:"warnings,omitempty"`
	Replaces     bool               `json:"replaces,omitempty"`
	Deployed     bool               `json:"deployed"`
}

// AppBundleImportOptions are the query flags of an import.
type AppBundleImportOptions struct {
	Name    string
	DryRun  bool
	Deploy  bool
	Replace bool
	Confirm string
}

// ImportAppBundle calls POST /api/v1/apps/import-bundle with a tar made by apps export.
func (c *Client) ImportAppBundle(ctx context.Context, bundle []byte, opts AppBundleImportOptions) (AppBundleImport, error) {
	q := url.Values{}
	if opts.Name != "" {
		q.Set("name", opts.Name)
	}
	for k, on := range map[string]bool{"dry_run": opts.DryRun, "deploy": opts.Deploy, "replace": opts.Replace} {
		if on {
			q.Set(k, "true")
		}
	}
	if opts.Confirm != "" {
		q.Set("confirm", opts.Confirm)
	}
	var out AppBundleImport
	_, err := c.doOnce(ctx, http.MethodPost, "/api/v1/apps/import-bundle?"+q.Encode(), bundle, &out)
	return out, err
}
