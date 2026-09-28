package provision

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

const hetznerBaseURL = "https://api.hetzner.cloud/v1"

// hetznerImage is the OS image every provisioned server boots: a current
// Ubuntu LTS, matching install.sh's own documented target for a fresh
// Linux box.
const hetznerImage = "ubuntu-24.04"

// Hetzner implements Provisioner against the Hetzner Cloud API
// (https://docs.hetzner.cloud), token auth, no SDK.
type Hetzner struct {
	client *httpClient
}

// NewHetzner returns a Hetzner provisioner authenticating with token.
func NewHetzner(token string) *Hetzner {
	return &Hetzner{client: newHTTPClient(hetznerBaseURL, token)}
}

type hetznerLocationsResponse struct {
	Locations []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"locations"`
}

// ListRegions calls GET /v1/locations.
func (h *Hetzner) ListRegions(ctx context.Context) ([]Region, error) {
	var out hetznerLocationsResponse
	if err := h.client.do(ctx, http.MethodGet, "/locations", nil, &out); err != nil {
		return nil, fmt.Errorf("provision: hetzner list regions: %w", err)
	}
	regions := make([]Region, 0, len(out.Locations))
	for _, l := range out.Locations {
		regions = append(regions, Region{ID: l.Name, Name: l.Description})
	}
	return regions, nil
}

type hetznerServerTypesResponse struct {
	ServerTypes []struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Cores       int     `json:"cores"`
		Memory      float64 `json:"memory"` // GB
		Disk        int     `json:"disk"`   // GB
		Deprecated  bool    `json:"deprecated"`
		Prices      []struct {
			Location     string `json:"location"`
			PriceMonthly struct {
				Gross string `json:"gross"`
			} `json:"price_monthly"`
		} `json:"prices"`
	} `json:"server_types"`
}

// ListSizes calls GET /v1/server_types. When region is non-empty, only
// server types priced in that location are returned (Hetzner's own signal
// for "available there"), with that location's own monthly price attached.
func (h *Hetzner) ListSizes(ctx context.Context, region string) ([]Size, error) {
	var out hetznerServerTypesResponse
	if err := h.client.do(ctx, http.MethodGet, "/server_types", nil, &out); err != nil {
		return nil, fmt.Errorf("provision: hetzner list sizes: %w", err)
	}
	var sizes []Size
	for _, st := range out.ServerTypes {
		if st.Deprecated {
			continue
		}
		size := Size{
			ID: st.Name, Name: st.Description, VCPUs: st.Cores,
			Memory: int(st.Memory * 1024), Disk: st.Disk, Currency: "EUR",
		}
		if region == "" {
			sizes = append(sizes, size)
			continue
		}
		for _, p := range st.Prices {
			if p.Location == region {
				size.PriceMonthly = p.PriceMonthly.Gross
				sizes = append(sizes, size)
				break
			}
		}
	}
	return sizes, nil
}

type hetznerCreateServerRequest struct {
	Name       string `json:"name"`
	ServerType string `json:"server_type"`
	Location   string `json:"location"`
	Image      string `json:"image"`
	UserData   string `json:"user_data"`
}

type hetznerServerResponse struct {
	Server hetznerServer `json:"server"`
}

type hetznerServer struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	PublicNet struct {
		IPv4 struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
	} `json:"public_net"`
}

// CreateServer calls POST /v1/servers.
func (h *Hetzner) CreateServer(ctx context.Context, opts CreateOpts) (serverID, ipAddr string, err error) {
	var out hetznerServerResponse
	req := hetznerCreateServerRequest{
		Name: opts.Name, ServerType: opts.Size, Location: opts.Region,
		Image: hetznerImage, UserData: opts.UserData,
	}
	if err := h.client.do(ctx, http.MethodPost, "/servers", req, &out); err != nil {
		return "", "", fmt.Errorf("provision: hetzner create server: %w", err)
	}
	return strconv.FormatInt(out.Server.ID, 10), out.Server.PublicNet.IPv4.IP, nil
}

// GetServer calls GET /v1/servers/{id}.
func (h *Hetzner) GetServer(ctx context.Context, id string) (ServerStatus, string, error) {
	var out hetznerServerResponse
	if err := h.client.do(ctx, http.MethodGet, "/servers/"+id, nil, &out); err != nil {
		return "", "", fmt.Errorf("provision: hetzner get server %q: %w", id, err)
	}
	return hetznerStatus(out.Server.Status), out.Server.PublicNet.IPv4.IP, nil
}

func hetznerStatus(s string) ServerStatus {
	switch s {
	case "running":
		return ServerStatusRunning
	case "off", "deleting":
		return ServerStatusError
	default:
		// initializing, starting, etc.
		return ServerStatusPending
	}
}

// DeleteServer calls DELETE /v1/servers/{id}.
func (h *Hetzner) DeleteServer(ctx context.Context, id string) error {
	if err := h.client.do(ctx, http.MethodDelete, "/servers/"+id, nil, nil); err != nil {
		return fmt.Errorf("provision: hetzner delete server %q: %w", id, err)
	}
	return nil
}
