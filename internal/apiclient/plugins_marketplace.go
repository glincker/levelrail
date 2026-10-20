package apiclient

import (
	"context"
	"net/http"
)

// MarketplaceEntry is one listing from a signed marketplace index.
type MarketplaceEntry struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Publisher        string   `json:"publisher"`
	PublisherKeyID   string   `json:"publisher_key_id"`
	PublisherKeyName string   `json:"publisher_key_name"`
	Version          string   `json:"version"`
	Summary          string   `json:"summary"`
	Categories       []string `json:"categories"`
	Icon             string   `json:"icon"`
	Abilities        []string `json:"abilities"`
	OutboundHosts    []string `json:"outbound_hosts"`
	Digest           string   `json:"digest"`
	Changelog        string   `json:"changelog"`
	MinVersion       string   `json:"min_version"`
	SourceID         string   `json:"source_id"`
	SourceName       string   `json:"source_name"`
	Verified         bool     `json:"verified"`
	KeyTrusted       bool     `json:"key_trusted"`
	Installed        bool     `json:"installed"`
	InstalledVersion string   `json:"installed_version"`
	UpdateAvailable  bool     `json:"update_available"`
	Compatible       bool     `json:"compatible"`
}

// MarketplaceSource is a configured index source with its last fetch outcome.
type MarketplaceSource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	RootKeyID   string `json:"root_key_id"`
	Builtin     bool   `json:"builtin"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	ErrorKind   string `json:"error_kind"`
	Sequence    int64  `json:"sequence"`
	GeneratedAt string `json:"generated_at"`
	FetchedAt   string `json:"fetched_at"`
	PluginCount int    `json:"plugin_count"`
}

// MarketplaceCatalog is every listing plus each source's status.
type MarketplaceCatalog struct {
	Entries []MarketplaceEntry  `json:"entries"`
	Sources []MarketplaceSource `json:"sources"`
}

// MarketplacePreview is the confirm sheet for one listing.
type MarketplacePreview struct {
	Entry            MarketplaceEntry `json:"entry"`
	SourceID         string           `json:"source_id"`
	SourceName       string           `json:"source_name"`
	KeyID            string           `json:"key_id"`
	KeyName          string           `json:"key_name"`
	KeyTrusted       bool             `json:"key_trusted"`
	Action           string           `json:"action"`
	InstalledVersion string           `json:"installed_version"`
	AddedAbilities   []string         `json:"added_abilities"`
	AddedHosts       []string         `json:"added_hosts"`
	Reapproval       bool             `json:"reapproval_required"`
}

// MarketplaceInstallInput confirms exactly what the sheet showed.
type MarketplaceInstallInput struct {
	SourceID   string   `json:"source_id"`
	ID         string   `json:"id"`
	Version    string   `json:"version"`
	Digest     string   `json:"digest"`
	Abilities  []string `json:"abilities"`
	TrustKeyID string   `json:"trust_key_id,omitempty"`
}

// MarketplaceInstallResult is what the one-click flow did.
type MarketplaceInstallResult struct {
	Plugin     Plugin `json:"plugin"`
	Action     string `json:"action"`
	TrustAdded bool   `json:"trust_added"`
}

type marketplaceSourceInput struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	RootKey string `json:"root_public_key"`
}

// MarketplaceCatalog calls GET /api/v1/plugin-marketplace.
func (c *Client) MarketplaceCatalog(ctx context.Context) (MarketplaceCatalog, error) {
	var out MarketplaceCatalog
	err := c.do(ctx, http.MethodGet, "/api/v1/plugin-marketplace", nil, &out)
	return out, err
}

// RefreshMarketplace calls POST /api/v1/plugin-marketplace/refresh. An empty sourceID refreshes every source.
func (c *Client) RefreshMarketplace(ctx context.Context, sourceID string) (MarketplaceCatalog, error) {
	var out MarketplaceCatalog
	err := c.do(ctx, http.MethodPost, "/api/v1/plugin-marketplace/refresh", map[string]string{"source_id": sourceID}, &out)
	return out, err
}

// PreviewMarketplacePlugin calls POST /api/v1/plugin-marketplace/preview.
func (c *Client) PreviewMarketplacePlugin(ctx context.Context, sourceID, id string) (MarketplacePreview, error) {
	var out MarketplacePreview
	err := c.do(ctx, http.MethodPost, "/api/v1/plugin-marketplace/preview", map[string]string{"source_id": sourceID, "id": id}, &out)
	return out, err
}

// InstallMarketplacePlugin calls POST /api/v1/plugin-marketplace/install.
func (c *Client) InstallMarketplacePlugin(ctx context.Context, in MarketplaceInstallInput) (MarketplaceInstallResult, error) {
	var out MarketplaceInstallResult
	err := c.do(ctx, http.MethodPost, "/api/v1/plugin-marketplace/install", in, &out)
	return out, err
}

// ListMarketplaceSources calls GET /api/v1/plugin-marketplace/sources.
func (c *Client) ListMarketplaceSources(ctx context.Context) ([]MarketplaceSource, error) {
	var out []MarketplaceSource
	err := c.do(ctx, http.MethodGet, "/api/v1/plugin-marketplace/sources", nil, &out)
	return out, err
}

// AddMarketplaceSource calls POST /api/v1/plugin-marketplace/sources.
func (c *Client) AddMarketplaceSource(ctx context.Context, name, url, rootKey string) (MarketplaceSource, error) {
	var out MarketplaceSource
	err := c.do(ctx, http.MethodPost, "/api/v1/plugin-marketplace/sources", marketplaceSourceInput{Name: name, URL: url, RootKey: rootKey}, &out)
	return out, err
}

// RemoveMarketplaceSource calls DELETE /api/v1/plugin-marketplace/sources/{id}.
func (c *Client) RemoveMarketplaceSource(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/plugin-marketplace/sources/"+PathEscape(id), nil, nil)
}
