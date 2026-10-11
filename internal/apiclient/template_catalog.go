package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ServiceTemplateFilter narrows GET /api/v1/service-templates.
type ServiceTemplateFilter struct {
	// Search matches id, name, slogan and category as a substring.
	Search      string
	Category    string
	Installable bool
}

// TemplateStackService mirrors catalog.StackService: one service of an
// installable template with its image, ports and named volumes.
type TemplateStackService struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Ports     []int    `json:"ports,omitempty"`
	Volumes   []string `json:"volumes,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// TemplateSetting mirrors one env prompt of an installable template.
type TemplateSetting struct {
	Key      string `json:"key"`
	Label    string `json:"label,omitempty"`
	Default  string `json:"default,omitempty"`
	Generate string `json:"generate,omitempty"`
	Length   int    `json:"length,omitempty"`
	Required bool   `json:"required,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
}

// TemplateDomainPrompt mirrors a service an installable template offers to put behind a domain.
type TemplateDomainPrompt struct {
	Service string `json:"service"`
	Port    int    `json:"port"`
}

// TemplateSource records where an imported template came from.
type TemplateSource struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	License string `json:"license"`
	Path    string `json:"path"`
}

// TemplateManifestInfo is the install-form part of a template detail.
type TemplateManifestInfo struct {
	Env     []TemplateSetting      `json:"env,omitempty"`
	Domains []TemplateDomainPrompt `json:"domains,omitempty"`
	Source  *TemplateSource        `json:"source,omitempty"`
}

// TemplateUpdateService is one service's result in a template update report.
type TemplateUpdateService struct {
	Service      string   `json:"service"`
	Image        string   `json:"image"`
	Status       string   `json:"status"`
	NewerTags    []string `json:"newer_tags,omitempty"`
	CatalogImage string   `json:"catalog_image,omitempty"`
	Note         string   `json:"note,omitempty"`
}

// TemplateUpdateApp is one installed template app in a template update report.
type TemplateUpdateApp struct {
	App        string                  `json:"app"`
	TemplateID string                  `json:"template_id"`
	Services   []TemplateUpdateService `json:"services"`
}

// TemplateUpdateReport is GET /api/v1/service-templates/updates.
type TemplateUpdateReport struct {
	Apps []TemplateUpdateApp `json:"apps"`
}

// SearchServiceTemplates calls GET /api/v1/service-templates with filters.
func (c *Client) SearchServiceTemplates(ctx context.Context, f ServiceTemplateFilter) ([]ServiceTemplateListItem, error) {
	q := url.Values{}
	if f.Search != "" {
		q.Set("q", f.Search)
	}
	if f.Category != "" {
		q.Set("category", f.Category)
	}
	if f.Installable {
		q.Set("installable", "true")
	}
	path := "/api/v1/service-templates"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []ServiceTemplateListItem
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// ServiceTemplateUpdates calls GET /api/v1/service-templates/updates:
// newer upstream image tags for every app installed from a template.
func (c *Client) ServiceTemplateUpdates(ctx context.Context) (TemplateUpdateReport, error) {
	var out TemplateUpdateReport
	err := c.do(ctx, http.MethodGet, "/api/v1/service-templates/updates", nil, &out)
	return out, err
}
