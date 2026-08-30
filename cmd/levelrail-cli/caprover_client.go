package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// caproverEnvVar mirrors one entry of a CapRover AppDefinition's envVars
// array (github.com/caprover/caprover, src/models/AppDefinition.ts).
// Unlike Coolify, CapRover's app-definition endpoint has no documented
// redaction gate: any caller that can list apps gets real values back
// here, the same trust model dokployApplication.Env already documents.
type caproverEnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// caproverVolume mirrors one entry of AppDefinition.volumes. HostPath is
// only set for a bind mount; a named volume leaves it empty.
type caproverVolume struct {
	ContainerPath string `json:"containerPath"`
	VolumeName    string `json:"volumeName"`
	HostPath      string `json:"hostPath,omitempty"`
}

// caproverPort mirrors one entry of AppDefinition.ports: a raw host<->
// container TCP/UDP mapping, separate from ContainerHTTPPort which
// AppDefinition.containerHttpPort/CapRover's ingress uses for HTTP routing.
type caproverPort struct {
	HostPort      int `json:"hostPort"`
	ContainerPort int `json:"containerPort"`
}

// caproverCustomDomain mirrors one entry of AppDefinition.customDomain.
type caproverCustomDomain struct {
	PublicDomain string `json:"publicDomain"`
	HasSsl       bool   `json:"hasSsl"`
}

// caproverAppDefinition mirrors the subset of CapRover's own AppDefinition
// model (src/models/AppDefinition.ts, returned by GET
// /api/v2/user/apps/appDefinitions) that field mapping reads. CapRover has
// no per-app CPU/memory limit feature at all, so unlike coolifyApplication
// and dokployApplication there is no limits field here to map.
//
// CaptainDefinitionRelativeFilePath points at a captain-definition file
// (schemaVersion, plus one of dockerfilePath/templateId/dockerfileLines)
// whose contents this read-only listing endpoint does not expose, so the
// actual build method cannot be determined from this API alone; see
// mapCaproverBuildConfig.
type caproverAppDefinition struct {
	AppName                           string                 `json:"appName"`
	HasPersistentData                 bool                   `json:"hasPersistentData"`
	InstanceCount                     int                    `json:"instanceCount"`
	CaptainDefinitionRelativeFilePath string                 `json:"captainDefinitionRelativeFilePath"`
	NotExposeAsWebApp                 bool                   `json:"notExposeAsWebApp"`
	ContainerHTTPPort                 int                    `json:"containerHttpPort"`
	EnvVars                           []caproverEnvVar       `json:"envVars"`
	Volumes                           []caproverVolume       `json:"volumes"`
	Ports                             []caproverPort         `json:"ports"`
	CustomDomain                      []caproverCustomDomain `json:"customDomain"`
}

// caproverEnvelope is the response shape every CapRover API call returns,
// success or failure alike, always under HTTP 200
// (src/api/ApiStatusCodes.ts): the real result lives in Status/Description,
// not the HTTP status line. Unverified against a live instance; see
// caprover_client.go's package-level assumptions in the migration report.
type caproverEnvelope struct {
	Status      int             `json:"status"`
	Description string          `json:"description"`
	Data        json.RawMessage `json:"data"`
}

// caproverStatusOK is ApiStatusCodes.STATUS_OK. CapRover also defines
// STATUS_OK_DEPLOY_STARTED (101) and STATUS_OK_PARTIALLY (102) for
// deploy-triggering endpoints this read-only client never calls.
const caproverStatusOK = 100

// caproverAPIError is returned for a non-OK caproverEnvelope.Status.
type caproverAPIError struct {
	Status      int
	Description string
}

func (e *caproverAPIError) Error() string {
	return fmt.Sprintf("caprover api returned status %d: %s", e.Status, e.Description)
}

// CaproverClient is a minimal, read-only client for CapRover's dashboard
// API. CapRover authenticates with a login exchange, not a static bearer
// token: POST /api/v2/login trades a password for a short-lived JWT
// (Login), sent back as the x-captain-auth header on every later request.
// This migration tool never writes back to the source CapRover instance.
type CaproverClient struct {
	baseURL  string
	password string
	token    string
	hc       *http.Client
}

// NewCaproverClient builds a CaproverClient. baseURL is the CapRover
// instance root, e.g. "https://captain.example.com"; "/api/v2" is
// appended per request. Call Login before any other method.
func NewCaproverClient(baseURL, password string) *CaproverClient {
	return &CaproverClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		password: password,
		hc:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Login exchanges c.password for a session token via POST
// /api/v2/login, storing it for every subsequent request's
// x-captain-auth header.
func (c *CaproverClient) Login(ctx context.Context) error {
	body, err := json.Marshal(struct { //nolint:gosec // encoding the operator-supplied --token value to send to caprover's own login endpoint, not a hardcoded credential
		Password string `json:"password"`
	}{c.password})
	if err != nil {
		return fmt.Errorf("encode login request: %w", err)
	}

	var data struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, http.MethodPost, "/login", bytes.NewReader(body), &data); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if data.Token == "" {
		return fmt.Errorf("login: caprover returned an empty token")
	}
	c.token = data.Token
	return nil
}

// caproverListAppsData is GET /api/v2/user/apps/appDefinitions's data
// payload. RootDomain is required to reconstruct an app's default
// subdomain (appName + "." + rootDomain), since AppDefinition itself
// carries no field for it.
type caproverListAppsData struct {
	AppDefinitions []caproverAppDefinition `json:"appDefinitions"`
	RootDomain     string                  `json:"rootDomain"`
}

// ListApplications calls GET /api/v2/user/apps/appDefinitions, which
// returns every app's full definition (env vars, volumes, ports, domains)
// in one call: unlike Coolify and Dokploy, CapRover has no separate
// per-app detail or per-app env/domain endpoint to fetch afterward.
func (c *CaproverClient) ListApplications(ctx context.Context) ([]caproverAppDefinition, string, error) {
	var data caproverListAppsData
	if err := c.do(ctx, http.MethodGet, "/user/apps/appDefinitions", nil, &data); err != nil {
		return nil, "", err
	}
	return data.AppDefinitions, data.RootDomain, nil
}

func (c *CaproverClient) do(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v2"+path, body) //nolint:gosec // baseURL is the operator-supplied --url target this command exists to call, not attacker-controlled input
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-namespace", "captain")
	if c.token != "" {
		req.Header.Set("x-captain-auth", c.token)
	}

	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	var env caproverEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("decode response envelope: %w", err)
	}
	if env.Status != caproverStatusOK {
		return &caproverAPIError{Status: env.Status, Description: env.Description}
	}

	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
	}
	return nil
}
