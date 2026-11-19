package apiclient

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

// StaticSiteSettings is a static site's stored overrides and what it serves now.
type StaticSiteSettings struct {
	Name          string               `json:"name"`
	BuildCommand  string               `json:"build_command"`
	OutputDir     string               `json:"output_dir"`
	SPAFallback   *bool                `json:"spa_fallback"`
	CleanURLs     *bool                `json:"clean_urls"`
	TrailingSlash string               `json:"trailing_slash"`
	NotFoundPage  string               `json:"not_found_page"`
	Protected     bool                 `json:"protected"`
	Effective     staticroutes.Options `json:"effective"`
}

// StaticSiteSettingsPatch changes only the fields set; Reset clears named overrides.
type StaticSiteSettingsPatch struct {
	BuildCommand  *string  `json:"build_command,omitempty"`
	OutputDir     *string  `json:"output_dir,omitempty"`
	SPAFallback   *bool    `json:"spa_fallback,omitempty"`
	CleanURLs     *bool    `json:"clean_urls,omitempty"`
	TrailingSlash *string  `json:"trailing_slash,omitempty"`
	NotFoundPage  *string  `json:"not_found_page,omitempty"`
	Reset         []string `json:"reset,omitempty"`
	// Password sets the gate password; a pointer to "" clears it.
	Password *string `json:"password,omitempty"`
}

// StaticSiteRulesBody is the editable rule set PUT accepts.
type StaticSiteRulesBody struct {
	Redirects []staticroutes.UIRedirect `json:"redirects"`
	Headers   []staticroutes.UIHeader   `json:"headers"`
}

// StaticSiteRuleRow is one effective redirect with its source (ui, app.yaml or file).
type StaticSiteRuleRow struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status int    `json:"status"`
	Force  bool   `json:"force"`
	Source string `json:"source"`
	Origin string `json:"origin"`
}

// StaticSiteHeaderRow is one effective header block with its source.
type StaticSiteHeaderRow struct {
	Path   string            `json:"path"`
	Set    map[string]string `json:"set"`
	Remove []string          `json:"remove"`
	Source string            `json:"source"`
	Origin string            `json:"origin"`
}

// StaticSiteRules is the effective ruleset plus the editable dashboard part.
type StaticSiteRules struct {
	Redirects []StaticSiteRuleRow    `json:"redirects"`
	Headers   []StaticSiteHeaderRow  `json:"headers"`
	Options   staticroutes.Options   `json:"options"`
	Problems  []staticroutes.Problem `json:"problems"`
	Published bool                   `json:"published"`
	UI        StaticSiteRulesBody    `json:"ui"`
}

// GetStaticSiteSettings calls GET /api/v1/static-sites/{name}/settings.
func (c *Client) GetStaticSiteSettings(ctx context.Context, name string) (StaticSiteSettings, error) {
	var out StaticSiteSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name)+"/settings", nil, &out)
	return out, err
}

// PatchStaticSiteSettings calls PATCH /api/v1/static-sites/{name}/settings.
func (c *Client) PatchStaticSiteSettings(ctx context.Context, name string, p StaticSiteSettingsPatch) (StaticSiteSettings, error) {
	var out StaticSiteSettings
	err := c.do(ctx, http.MethodPatch, "/api/v1/static-sites/"+PathEscape(name)+"/settings", p, &out)
	return out, err
}

// GetStaticSiteRules calls GET /api/v1/static-sites/{name}/rules.
func (c *Client) GetStaticSiteRules(ctx context.Context, name string) (StaticSiteRules, error) {
	var out StaticSiteRules
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name)+"/rules", nil, &out)
	return out, err
}

// PutStaticSiteRules calls PUT /api/v1/static-sites/{name}/rules.
func (c *Client) PutStaticSiteRules(ctx context.Context, name string, body StaticSiteRulesBody) (StaticSiteRules, error) {
	var out StaticSiteRules
	err := c.do(ctx, http.MethodPut, "/api/v1/static-sites/"+PathEscape(name)+"/rules", body, &out)
	return out, err
}
