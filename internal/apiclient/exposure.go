package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ExposureOwner mirrors internal/exposure.Owner.
type ExposureOwner struct {
	Kind        string `json:"kind"`
	Name        string `json:"name,omitempty"`
	Intentional bool   `json:"intentional,omitempty"`
}

// ExposureOutsideCheck mirrors internal/api's exposureOutsideResource.
type ExposureOutsideCheck struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// ExposureRestriction mirrors internal/api's exposureRestrictionResource.
type ExposureRestriction struct {
	Allow     []string `json:"allow"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// ExposureFinding mirrors internal/api's exposureFindingResource.
type ExposureFinding struct {
	Container         string               `json:"container"`
	Image             string               `json:"image"`
	Owner             ExposureOwner        `json:"owner"`
	ImageKind         string               `json:"image_kind"`
	Protocol          string               `json:"protocol"`
	HostPort          int                  `json:"host_port"`
	ContainerPort     int                  `json:"container_port"`
	Binds             []string             `json:"binds"`
	Class             string               `json:"class"`
	Severity          string               `json:"severity"`
	AllowedSources    []string             `json:"allowed_sources,omitempty"`
	Rules             []string             `json:"rules,omitempty"`
	Managed           bool                 `json:"managed"`
	Explanation       string               `json:"explanation"`
	Recommendation    string               `json:"recommendation,omitempty"`
	CanRestrict       bool                 `json:"can_restrict"`
	CannotRestrictWhy string               `json:"cannot_restrict_reason,omitempty"`
	Restriction       *ExposureRestriction `json:"restriction,omitempty"`
	Outside           ExposureOutsideCheck `json:"outside_check"`
}

// ExposureNode mirrors internal/api's exposureNodeResource.
type ExposureNode struct {
	NodeID        string            `json:"node_id"`
	NodeName      string            `json:"node_name"`
	Local         bool              `json:"local"`
	Status        string            `json:"status"`
	Error         string            `json:"error,omitempty"`
	RulesReadable bool              `json:"rules_readable"`
	RulesNote     string            `json:"rules_note,omitempty"`
	PublicAddress string            `json:"public_address,omitempty"`
	Findings      []ExposureFinding `json:"findings"`
}

// ExposureReport mirrors internal/api's exposureReportResource.
type ExposureReport struct {
	GeneratedAt string         `json:"generated_at"`
	Nodes       []ExposureNode `json:"nodes"`
	Exposed     int            `json:"exposed"`
	High        int            `json:"high"`
}

// ExposureRestrictRequest mirrors internal/api's exposureRestrictRequest.
type ExposureRestrictRequest struct {
	Node            string   `json:"node,omitempty"`
	Port            int      `json:"port,omitempty"`
	Protocol        string   `json:"protocol,omitempty"`
	Allow           []string `json:"allow"`
	LocalContainers bool     `json:"local_containers,omitempty"`
	Confirm         bool     `json:"confirm,omitempty"`
}

// ExposurePlan mirrors internal/api's exposurePlanResource.
type ExposurePlan struct {
	Commands    []string `json:"commands"`
	Drops       string   `json:"drops"`
	Tag         string   `json:"tag"`
	Persistence string   `json:"persistence"`
	Port        int      `json:"port"`
	Protocol    string   `json:"protocol"`
	Allow       []string `json:"allow"`
	Warnings    []string `json:"warnings"`
	Applied     bool     `json:"applied"`
}

// GetExposure calls GET /api/v1/firewall/exposure.
func (c *Client) GetExposure(ctx context.Context, node string, probe bool) (ExposureReport, error) {
	q := url.Values{}
	if node != "" {
		q.Set("node", node)
	}
	if probe {
		q.Set("probe", "true")
	}
	path := "/api/v1/firewall/exposure"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out ExposureReport
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// PreviewExposureRestriction calls POST /api/v1/firewall/exposure/preview.
func (c *Client) PreviewExposureRestriction(ctx context.Context, req ExposureRestrictRequest) (ExposurePlan, error) {
	var out ExposurePlan
	err := c.do(ctx, http.MethodPost, "/api/v1/firewall/exposure/preview", req, &out)
	return out, err
}

func exposureRestrictionPath(proto string, port int) string {
	if proto == "" {
		proto = "tcp"
	}
	return "/api/v1/firewall/exposure/restrictions/" + PathEscape(proto) + "/" + strconv.Itoa(port)
}

// ApplyExposureRestriction calls PUT .../exposure/restrictions/{protocol}/{port}.
func (c *Client) ApplyExposureRestriction(ctx context.Context, req ExposureRestrictRequest) (ExposurePlan, error) {
	var out ExposurePlan
	err := c.do(ctx, http.MethodPut, exposureRestrictionPath(req.Protocol, req.Port), req, &out)
	return out, err
}

// RemoveExposureRestriction calls DELETE .../exposure/restrictions/{protocol}/{port}.
func (c *Client) RemoveExposureRestriction(ctx context.Context, protocol string, port int) error {
	return c.do(ctx, http.MethodDelete, exposureRestrictionPath(protocol, port), nil, nil)
}
