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

// CapRover returns real env values to any caller that can list apps, with no redaction gate.
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

// CapRover has no per-app CPU or memory limits, so no limits field is mapped.
// The build method is not in this response, so see mapCaproverBuildConfig.
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

// Every CapRover response is HTTP 200; the real result is in Status and Description.
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

// CaproverClient is a read-only client for CapRover's dashboard API, authenticated by a JWT login.
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

// RootDomain rebuilds each app's default subdomain, since AppDefinition has no field for it.
type caproverListAppsData struct {
	AppDefinitions []caproverAppDefinition `json:"appDefinitions"`
	RootDomain     string                  `json:"rootDomain"`
}

// ListApplications returns every app's full definition in one call; CapRover has no per-app endpoint.
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
