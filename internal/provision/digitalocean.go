package provision

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

const digitaloceanBaseURL = "https://api.digitalocean.com/v2"

// digitaloceanImage is the OS image every provisioned droplet boots: a
// current Ubuntu LTS, matching hetznerImage's own choice.
const digitaloceanImage = "ubuntu-24-04-x64"

// DigitalOcean implements Provisioner against the DigitalOcean API v2
// (https://docs.digitalocean.com/reference/api/), token auth, no SDK.
type DigitalOcean struct {
	client *httpClient
}

// NewDigitalOcean returns a DigitalOcean provisioner authenticating with
// token.
func NewDigitalOcean(token string) *DigitalOcean {
	return &DigitalOcean{client: newHTTPClient(digitaloceanBaseURL, token)}
}

type doRegionsResponse struct {
	Regions []struct {
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		Available bool   `json:"available"`
	} `json:"regions"`
}

// ListRegions calls GET /v2/regions.
func (d *DigitalOcean) ListRegions(ctx context.Context) ([]Region, error) {
	var out doRegionsResponse
	if err := d.client.do(ctx, http.MethodGet, "/regions", nil, &out); err != nil {
		return nil, fmt.Errorf("provision: digitalocean list regions: %w", err)
	}
	regions := make([]Region, 0, len(out.Regions))
	for _, r := range out.Regions {
		if !r.Available {
			continue
		}
		regions = append(regions, Region{ID: r.Slug, Name: r.Name})
	}
	return regions, nil
}

type doSizesResponse struct {
	Sizes []struct {
		Slug         string   `json:"slug"`
		Memory       int      `json:"memory"` // MB
		VCPUs        int      `json:"vcpus"`
		Disk         int      `json:"disk"` // GB
		PriceMonthly float64  `json:"price_monthly"`
		Regions      []string `json:"regions"`
		Available    bool     `json:"available"`
	} `json:"sizes"`
}

// ListSizes calls GET /v2/sizes. When region is non-empty, only sizes
// DigitalOcean lists as available in that region are returned.
func (d *DigitalOcean) ListSizes(ctx context.Context, region string) ([]Size, error) {
	var out doSizesResponse
	if err := d.client.do(ctx, http.MethodGet, "/sizes", nil, &out); err != nil {
		return nil, fmt.Errorf("provision: digitalocean list sizes: %w", err)
	}
	var sizes []Size
	for _, s := range out.Sizes {
		if !s.Available {
			continue
		}
		if region != "" && !containsString(s.Regions, region) {
			continue
		}
		sizes = append(sizes, Size{
			ID: s.Slug, Name: s.Slug, VCPUs: s.VCPUs, Memory: s.Memory, Disk: s.Disk,
			PriceMonthly: strconv.FormatFloat(s.PriceMonthly, 'f', 2, 64), Currency: "USD",
		})
	}
	return sizes, nil
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

type doCreateDropletRequest struct {
	Name     string `json:"name"`
	Region   string `json:"region"`
	Size     string `json:"size"`
	Image    string `json:"image"`
	UserData string `json:"user_data"`
}

type doDropletResponse struct {
	Droplet doDroplet `json:"droplet"`
}

type doDroplet struct {
	ID       int64  `json:"id"`
	Status   string `json:"status"`
	Networks struct {
		V4 []struct {
			IPAddress string `json:"ip_address"`
			Type      string `json:"type"`
		} `json:"v4"`
	} `json:"networks"`
}

func (d doDroplet) publicIPv4() string {
	for _, n := range d.Networks.V4 {
		if n.Type == "public" {
			return n.IPAddress
		}
	}
	return ""
}

// CreateServer calls POST /v2/droplets.
func (d *DigitalOcean) CreateServer(ctx context.Context, opts CreateOpts) (serverID, ipAddr string, err error) {
	var out doDropletResponse
	req := doCreateDropletRequest{
		Name: opts.Name, Region: opts.Region, Size: opts.Size,
		Image: digitaloceanImage, UserData: opts.UserData,
	}
	if err := d.client.do(ctx, http.MethodPost, "/droplets", req, &out); err != nil {
		return "", "", fmt.Errorf("provision: digitalocean create droplet: %w", err)
	}
	return strconv.FormatInt(out.Droplet.ID, 10), out.Droplet.publicIPv4(), nil
}

// GetServer calls GET /v2/droplets/{id}.
func (d *DigitalOcean) GetServer(ctx context.Context, id string) (ServerStatus, string, error) {
	var out doDropletResponse
	if err := d.client.do(ctx, http.MethodGet, "/droplets/"+id, nil, &out); err != nil {
		return "", "", fmt.Errorf("provision: digitalocean get droplet %q: %w", id, err)
	}
	return digitaloceanStatus(out.Droplet.Status), out.Droplet.publicIPv4(), nil
}

func digitaloceanStatus(s string) ServerStatus {
	switch s {
	case "active":
		return ServerStatusRunning
	case "archive", "off":
		return ServerStatusError
	default:
		// new
		return ServerStatusPending
	}
}

// DeleteServer calls DELETE /v2/droplets/{id}.
func (d *DigitalOcean) DeleteServer(ctx context.Context, id string) error {
	if err := d.client.do(ctx, http.MethodDelete, "/droplets/"+id, nil, nil); err != nil {
		return fmt.Errorf("provision: digitalocean delete droplet %q: %w", id, err)
	}
	return nil
}
