package apiclient

import (
	"context"
	"net/http"
)

// Plugin is one installed plugin as the control plane reports it.
type Plugin struct {
	Name               string          `json:"name"`
	Version            string          `json:"version"`
	Kind               string          `json:"kind"`
	Publisher          string          `json:"publisher"`
	Description        string          `json:"description"`
	Image              string          `json:"image"`
	Digest             string          `json:"digest"`
	Manifest           string          `json:"manifest"`
	KeyID              string          `json:"key_id"`
	RequestedAbilities []string        `json:"requested_abilities"`
	ApprovedAbilities  []string        `json:"approved_abilities"`
	OutboundHosts      []string        `json:"outbound_hosts"`
	ConfigJSON         string          `json:"config_json"`
	State              string          `json:"state"`
	Running            bool            `json:"running"`
	Health             string          `json:"health"`
	Verification       string          `json:"verification"`
	Fix                string          `json:"fix"`
	Panels             []PluginPanel   `json:"panels"`
	Commands           []PluginCommand `json:"commands"`
	ApprovedAt         string          `json:"approved_at"`
	ApprovedBy         string          `json:"approved_by"`
	InstalledAt        string          `json:"installed_at"`
	InstalledBy        string          `json:"installed_by"`
	LastError          string          `json:"last_error"`
}

// PluginPanel is a read-only card a plugin contributes through its manifest.
type PluginPanel struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Rows        []PluginPanelRow `json:"rows"`
	Slot        string           `json:"slot"`
}

// PluginPanelRow is one label and plain-text value in a PluginPanel.
type PluginPanelRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// PluginVerification is what the control plane could prove about a package.
type PluginVerification struct {
	Status       string `json:"status"`
	DigestPinned bool   `json:"digest_pinned"`
	Digest       string `json:"digest"`
	Signed       bool   `json:"signed"`
	KeyID        string `json:"key_id"`
	KeyName      string `json:"key_name"`
	Installable  bool   `json:"installable"`
	Reason       string `json:"reason"`
}

// PluginManifest is the parsed plugin.yaml in a preview.
type PluginManifest struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Kind          string   `json:"kind"`
	Publisher     string   `json:"publisher"`
	Description   string   `json:"description"`
	Image         string   `json:"image"`
	Abilities     []string `json:"abilities"`
	OutboundHosts []string `json:"outbound_hosts"`
}

// PluginPreview is the review shown before an install or upgrade is approved.
type PluginPreview struct {
	Manifest           PluginManifest     `json:"manifest"`
	Verification       PluginVerification `json:"verification"`
	Action             string             `json:"action"`
	InstalledVersion   string             `json:"installed_version"`
	ApprovedAbilities  []string           `json:"approved_abilities"`
	AddedAbilities     []string           `json:"added_abilities"`
	RemovedAbilities   []string           `json:"removed_abilities"`
	AddedHosts         []string           `json:"added_hosts"`
	RemovedHosts       []string           `json:"removed_hosts"`
	ReapprovalRequired bool               `json:"reapproval_required"`
}

// PluginKey is one trusted publisher key.
type PluginKey struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	AddedAt   string `json:"added_at"`
	AddedBy   string `json:"added_by"`
}

// InstallPluginInput is the package an operator submits for install.
type InstallPluginInput struct {
	Manifest         string   `json:"manifest,omitempty"`
	ManifestURL      string   `json:"manifest_url,omitempty"`
	Signature        string   `json:"signature,omitempty"`
	SignatureURL     string   `json:"signature_url,omitempty"`
	KeyID            string   `json:"key_id,omitempty"`
	ExpectedDigest   string   `json:"expected_digest,omitempty"`
	ConfigJSON       string   `json:"config_json,omitempty"`
	ApproveAbilities []string `json:"approve_abilities,omitempty"`
}

type approvePluginInput struct {
	Abilities []string `json:"abilities"`
}

type addPluginKeyInput struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

// ListPlugins calls GET /api/v1/plugins.
func (c *Client) ListPlugins(ctx context.Context) ([]Plugin, error) {
	var out []Plugin
	err := c.do(ctx, http.MethodGet, "/api/v1/plugins", nil, &out)
	return out, err
}

// GetPlugin calls GET /api/v1/plugins/{name}.
func (c *Client) GetPlugin(ctx context.Context, name string) (Plugin, error) {
	var out Plugin
	err := c.do(ctx, http.MethodGet, "/api/v1/plugins/"+PathEscape(name), nil, &out)
	return out, err
}

// InstallPlugin calls POST /api/v1/plugins. The plugin is recorded but not approved or started.
func (c *Client) InstallPlugin(ctx context.Context, in InstallPluginInput) (Plugin, error) {
	var out Plugin
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins", in, &out)
	return out, err
}

// ApprovePlugin calls POST /api/v1/plugins/{name}/approve.
func (c *Client) ApprovePlugin(ctx context.Context, name string, abilities []string) (Plugin, error) {
	var out Plugin
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins/"+PathEscape(name)+"/approve", approvePluginInput{Abilities: abilities}, &out)
	return out, err
}

// EnablePlugin calls POST /api/v1/plugins/{name}/enable.
func (c *Client) EnablePlugin(ctx context.Context, name string) (Plugin, error) {
	var out Plugin
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins/"+PathEscape(name)+"/enable", nil, &out)
	return out, err
}

// DisablePlugin calls POST /api/v1/plugins/{name}/disable.
func (c *Client) DisablePlugin(ctx context.Context, name string) (Plugin, error) {
	var out Plugin
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins/"+PathEscape(name)+"/disable", nil, &out)
	return out, err
}

// RemovePlugin calls DELETE /api/v1/plugins/{name}.
func (c *Client) RemovePlugin(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/plugins/"+PathEscape(name), nil, nil)
}

// ListPluginKeys calls GET /api/v1/plugin-keys.
func (c *Client) ListPluginKeys(ctx context.Context) ([]PluginKey, error) {
	var out []PluginKey
	err := c.do(ctx, http.MethodGet, "/api/v1/plugin-keys", nil, &out)
	return out, err
}

// AddPluginKey calls POST /api/v1/plugin-keys.
func (c *Client) AddPluginKey(ctx context.Context, name, publicKey string) (PluginKey, error) {
	var out PluginKey
	err := c.do(ctx, http.MethodPost, "/api/v1/plugin-keys", addPluginKeyInput{Name: name, PublicKey: publicKey}, &out)
	return out, err
}

// RemovePluginKey calls DELETE /api/v1/plugin-keys/{id}.
func (c *Client) RemovePluginKey(ctx context.Context, keyID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/plugin-keys/"+PathEscape(keyID), nil, nil)
}

// InspectPlugin calls POST /api/v1/plugins/inspect. Nothing is installed.
func (c *Client) InspectPlugin(ctx context.Context, in InstallPluginInput) (PluginPreview, error) {
	var out PluginPreview
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins/inspect", in, &out)
	return out, err
}

// UpgradePlugin calls POST /api/v1/plugins/{name}/upgrade.
func (c *Client) UpgradePlugin(ctx context.Context, name string, in InstallPluginInput) (Plugin, error) {
	var out Plugin
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins/"+PathEscape(name)+"/upgrade", in, &out)
	return out, err
}

// RestartPlugin calls POST /api/v1/plugins/{name}/restart.
func (c *Client) RestartPlugin(ctx context.Context, name string) (Plugin, error) {
	var out Plugin
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins/"+PathEscape(name)+"/restart", nil, &out)
	return out, err
}
