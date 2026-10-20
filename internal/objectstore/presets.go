// Package objectstore is the S3-compatible storage layer shared by log
// archive and other features that write to a connected storage destination.
package objectstore

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Preset identifies a well-known S3-compatible provider.
const (
	PresetAWS     = "aws"
	PresetR2      = "r2"
	PresetB2      = "b2"
	PresetMinIO   = "minio"
	PresetWasabi  = "wasabi"
	PresetCustom  = "custom"
	defaultRegion = "us-east-1"
)

// Preset describes how a provider's endpoint, region and addressing work.
type Preset struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// EndpointTemplate may contain {account_id} and {region}. Empty means the
	// SDK resolves the endpoint (AWS) or the caller supplies one.
	EndpointTemplate string `json:"endpoint_template,omitempty"`
	DefaultRegion    string `json:"default_region,omitempty"`
	PathStyle        bool   `json:"path_style"`
	NeedsAccountID   bool   `json:"needs_account_id"`
	NeedsRegion      bool   `json:"needs_region"`
	NeedsEndpoint    bool   `json:"needs_endpoint"`
	// Provider is the backup target provider this preset is stored as.
	Provider string `json:"provider"`
}

var presets = []Preset{
	{ID: PresetAWS, Label: "AWS S3", DefaultRegion: defaultRegion, NeedsRegion: true, Provider: "aws"},
	{ID: PresetR2, Label: "Cloudflare R2", EndpointTemplate: "https://{account_id}.r2.cloudflarestorage.com", DefaultRegion: "auto", PathStyle: true, NeedsAccountID: true, Provider: "r2"},
	{ID: PresetB2, Label: "Backblaze B2", EndpointTemplate: "https://s3.{region}.backblazeb2.com", NeedsRegion: true, PathStyle: true, Provider: "custom"},
	{ID: PresetWasabi, Label: "Wasabi", EndpointTemplate: "https://s3.{region}.wasabisys.com", DefaultRegion: defaultRegion, NeedsRegion: true, PathStyle: true, Provider: "custom"},
	{ID: PresetMinIO, Label: "MinIO", DefaultRegion: defaultRegion, PathStyle: true, NeedsEndpoint: true, Provider: "custom"},
	{ID: PresetCustom, Label: "S3 compatible", DefaultRegion: defaultRegion, PathStyle: true, NeedsEndpoint: true, Provider: "custom"},
}

// Presets returns the provider presets in display order.
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// PresetByID looks up a preset.
func PresetByID(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// PresetForProvider infers a preset for a backup target created without one.
func PresetForProvider(provider string) string {
	switch provider {
	case "aws":
		return PresetAWS
	case "r2":
		return PresetR2
	default:
		return PresetCustom
	}
}

// Resolved is a preset applied to user input.
type Resolved struct {
	Endpoint string
	Region   string
}

// Resolve applies a preset to the user's endpoint, region and account ID.
func Resolve(presetID, endpoint, region, accountID string) (Resolved, error) {
	p, ok := PresetByID(presetID)
	if !ok {
		return Resolved{}, fmt.Errorf("unknown provider preset %q", presetID)
	}
	endpoint = strings.TrimSpace(endpoint)
	region = strings.TrimSpace(region)
	accountID = strings.TrimSpace(accountID)
	if region == "" {
		region = p.DefaultRegion
	}
	if p.NeedsRegion && region == "" {
		return Resolved{}, fmt.Errorf("region is required for %s", p.Label)
	}
	if p.NeedsAccountID && accountID == "" && endpoint == "" {
		return Resolved{}, fmt.Errorf("account id is required for %s", p.Label)
	}
	if endpoint == "" && p.EndpointTemplate != "" {
		endpoint = strings.NewReplacer("{account_id}", accountID, "{region}", region).Replace(p.EndpointTemplate)
	}
	if endpoint == "" && p.NeedsEndpoint {
		return Resolved{}, fmt.Errorf("endpoint is required for %s", p.Label)
	}
	if endpoint != "" {
		if err := ValidateEndpoint(endpoint); err != nil {
			return Resolved{}, err
		}
	}
	return Resolved{Endpoint: strings.TrimRight(endpoint, "/"), Region: region}, nil
}

// ValidateEndpoint checks scheme and host shape. Internal addresses are
// refused at dial time by netguard, not here, so DNS tricks cannot bypass it.
func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("endpoint is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("endpoint must start with http:// or https://")
	}
	if u.Host == "" {
		return errors.New("endpoint is missing a host")
	}
	if u.User != nil {
		return errors.New("endpoint must not embed credentials")
	}
	return nil
}
