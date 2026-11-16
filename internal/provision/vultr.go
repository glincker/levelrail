package provision

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const vultrBaseURL = "https://api.vultr.com/v2"

// vultrMaxPages bounds cursor pagination so a misbehaving server cannot
// loop a catalog request forever.
const vultrMaxPages = 20

// vultrUnassignedIP is what Vultr reports as main_ip until the address exists.
const vultrUnassignedIP = "0.0.0.0"

// vultrIDRe is the shape of an instance id (a UUID). It keeps an empty or
// odd stored id from turning a delete into a different route.
var vultrIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// vultrUbuntuRe picks "Ubuntu 24.04 LTS x64" style names out of /os.
var vultrUbuntuRe = regexp.MustCompile(`^Ubuntu (\d+)\.(\d+) LTS x64$`)

// Vultr implements Provisioner against the Vultr API v2
// (https://www.vultr.com/api/), bearer key auth, no SDK.
type Vultr struct {
	client *httpClient
	key    string
}

// NewVultr returns a Vultr provisioner authenticating with the API key.
func NewVultr(apiKey string) *Vultr {
	return &Vultr{client: newHTTPClient(vultrBaseURL, apiKey), key: apiKey}
}

// vultrError keeps the HTTP status but drops the provider body and any
// trace of the key, so nothing sensitive reaches logs or API responses.
func (v *Vultr) vultrError(op string, err error) error {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return fmt.Errorf("provision: vultr %s: %w", op, &ProviderError{Status: pe.Status})
	}
	if v.key != "" && strings.Contains(err.Error(), v.key) {
		return fmt.Errorf("provision: vultr %s: %s", op, strings.ReplaceAll(err.Error(), v.key, "[redacted]"))
	}
	return fmt.Errorf("provision: vultr %s: %w", op, err)
}

type vultrMeta struct {
	Meta struct {
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	} `json:"meta"`
}

// vultrPages walks every cursor page of a list endpoint, calling each with
// the decoded page; each returns the next cursor from the same page.
func vultrPages[T any](ctx context.Context, v *Vultr, op, path string, next func(*T) string, each func(*T)) error {
	cursor := ""
	for i := 0; i < vultrMaxPages; i++ {
		p := path + "?per_page=500"
		if cursor != "" {
			p += "&cursor=" + url.QueryEscape(cursor)
		}
		var out T
		if err := v.client.do(ctx, http.MethodGet, p, nil, &out); err != nil {
			return v.vultrError(op, err)
		}
		each(&out)
		if cursor = next(&out); cursor == "" {
			return nil
		}
	}
	return nil
}

type vultrRegionsResponse struct {
	Regions []struct {
		ID      string `json:"id"`
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"regions"`
	vultrMeta
}

// ListRegions calls GET /v2/regions, following every page.
func (v *Vultr) ListRegions(ctx context.Context) ([]Region, error) {
	var regions []Region
	err := vultrPages(ctx, v, "list regions", "/regions",
		func(r *vultrRegionsResponse) string { return r.Meta.Links.Next },
		func(r *vultrRegionsResponse) {
			for _, x := range r.Regions {
				regions = append(regions, Region{ID: x.ID, Name: x.City + " (" + x.Country + ")"})
			}
		})
	return regions, err
}

type vultrPlansResponse struct {
	Plans []struct {
		ID          string   `json:"id"`
		VCPUCount   int      `json:"vcpu_count"`
		RAM         int      `json:"ram"`  // MB
		Disk        int      `json:"disk"` // GB
		MonthlyCost float64  `json:"monthly_cost"`
		Locations   []string `json:"locations"`
	} `json:"plans"`
	vultrMeta
}

// ListSizes calls GET /v2/plans. Each plan carries its locations, so a
// non-empty region keeps only plans Vultr offers there, and Size.Regions
// feeds the size-region check.
func (v *Vultr) ListSizes(ctx context.Context, region string) ([]Size, error) {
	var sizes []Size
	err := vultrPages(ctx, v, "list plans", "/plans",
		func(r *vultrPlansResponse) string { return r.Meta.Links.Next },
		func(r *vultrPlansResponse) {
			for _, p := range r.Plans {
				if region != "" && !containsString(p.Locations, region) {
					continue
				}
				sizes = append(sizes, Size{
					ID: p.ID, Name: p.ID, VCPUs: p.VCPUCount, Memory: p.RAM, Disk: p.Disk,
					PriceMonthly: strconv.FormatFloat(p.MonthlyCost, 'f', 2, 64), Currency: "USD",
					Regions: p.Locations,
				})
			}
		})
	return sizes, err
}

type vultrOSResponse struct {
	OS []struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Family string `json:"family"`
	} `json:"os"`
	vultrMeta
}

// ubuntuLTSImage resolves the newest "Ubuntu x.y LTS x64" os id from the
// catalog, since Vultr os ids are numbers that change between releases.
func (v *Vultr) ubuntuLTSImage(ctx context.Context) (int, error) {
	best, bestVer := 0, 0
	err := vultrPages(ctx, v, "list os images", "/os",
		func(r *vultrOSResponse) string { return r.Meta.Links.Next },
		func(r *vultrOSResponse) {
			for _, o := range r.OS {
				m := vultrUbuntuRe.FindStringSubmatch(o.Name)
				if m == nil {
					continue
				}
				major, _ := strconv.Atoi(m[1])
				minor, _ := strconv.Atoi(m[2])
				if ver := major*100 + minor; ver > bestVer {
					best, bestVer = o.ID, ver
				}
			}
		})
	if err != nil {
		return 0, err
	}
	if best == 0 {
		return 0, errors.New("provision: vultr: no Ubuntu LTS image in the catalog")
	}
	return best, nil
}

type vultrCreateRequest struct {
	Region   string   `json:"region"`
	Plan     string   `json:"plan"`
	OSID     int      `json:"os_id"`
	Label    string   `json:"label"`
	Hostname string   `json:"hostname"`
	UserData string   `json:"user_data"`
	Tags     []string `json:"tags,omitempty"`
	Backups  string   `json:"backups"`
}

type vultrInstanceResponse struct {
	Instance struct {
		ID          string `json:"id"`
		MainIP      string `json:"main_ip"`
		Status      string `json:"status"`
		PowerStatus string `json:"power_status"`
	} `json:"instance"`
}

func vultrIP(ip string) string {
	if ip == vultrUnassignedIP {
		return ""
	}
	return ip
}

// vultrTags marks an instance as Levelrail-made and ties it to its
// provision row so orphans are findable.
func vultrTags(provisionID string) []string {
	tags := []string{"levelrail"}
	if provisionID != "" {
		tags = append(tags, "levelrail-provision-"+provisionID)
	}
	return tags
}

// CreateServer calls POST /v2/instances with base64 user_data.
func (v *Vultr) CreateServer(ctx context.Context, opts CreateOpts) (serverID, ipAddr string, err error) {
	osID, err := v.ubuntuLTSImage(ctx)
	if err != nil {
		return "", "", err
	}
	req := vultrCreateRequest{
		Region: opts.Region, Plan: opts.Size, OSID: osID, Label: opts.Name, Hostname: opts.Name,
		UserData: base64.StdEncoding.EncodeToString([]byte(opts.UserData)),
		Tags:     vultrTags(opts.ProvisionID), Backups: "disabled",
	}
	var out vultrInstanceResponse
	if err := v.client.do(ctx, http.MethodPost, "/instances", req, &out); err != nil {
		return "", "", v.vultrError("create instance", err)
	}
	return out.Instance.ID, vultrIP(out.Instance.MainIP), nil
}

// GetServer calls GET /v2/instances/{id}.
func (v *Vultr) GetServer(ctx context.Context, id string) (ServerStatus, string, error) {
	if !vultrIDRe.MatchString(id) {
		return "", "", errors.New("provision: vultr get instance: invalid instance id")
	}
	var out vultrInstanceResponse
	if err := v.client.do(ctx, http.MethodGet, "/instances/"+id, nil, &out); err != nil {
		return "", "", v.vultrError("get instance", err)
	}
	ip := vultrIP(out.Instance.MainIP)
	return vultrStatus(out.Instance.Status, out.Instance.PowerStatus, ip), ip, nil
}

func vultrStatus(status, power, ip string) ServerStatus {
	switch {
	case status == "suspended" || power == "stopped":
		return ServerStatusError
	case status == "active" && power == "running" && ip != "":
		return ServerStatusRunning
	default:
		return ServerStatusPending
	}
}

// DeleteServer calls DELETE /v2/instances/{id} for the stored id only. A
// 404 means it is already gone, which is success for a retried destroy.
func (v *Vultr) DeleteServer(ctx context.Context, id string) error {
	if !vultrIDRe.MatchString(id) {
		return errors.New("provision: vultr delete instance: invalid instance id")
	}
	err := v.client.do(ctx, http.MethodDelete, "/instances/"+id, nil, nil)
	var pe *ProviderError
	if errors.As(err, &pe) && pe.Status == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return v.vultrError("delete instance", err)
	}
	return nil
}
