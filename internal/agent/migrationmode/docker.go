package migrationmode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Docker is a read-only Engine API client. Every request goes through the
// allowlist guard, so even a bug in this file cannot reach a write endpoint.
type Docker struct {
	hc    *http.Client
	limit int64
}

// NewDocker connects to the Docker unix socket at socket. maxResponse caps the
// bytes read from any single response.
func NewDocker(socket string, maxResponse int64, onDeny DenyFunc) *Docker {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	base := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socket)
	}}
	return NewDockerWithTransport(base, maxResponse, onDeny)
}

// NewDockerWithTransport is NewDocker over an arbitrary transport, for tests.
func NewDockerWithTransport(next http.RoundTripper, maxResponse int64, onDeny DenyFunc) *Docker {
	return &Docker{hc: &http.Client{Transport: NewGuard(next, onDeny)}, limit: maxResponse}
}

func (d *Docker) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, http.NoBody)
	if err != nil {
		return fmt.Errorf("migrationmode: build request %s: %w", path, err)
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrationmode: docker %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("migrationmode: docker %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, d.limit+1))
	if err != nil {
		return fmt.Errorf("migrationmode: read docker %s: %w", path, err)
	}
	if int64(len(body)) > d.limit {
		return fmt.Errorf("migrationmode: docker %s: response over %d bytes", path, d.limit)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("migrationmode: decode docker %s: %w", path, err)
	}
	return nil
}

type apiVersion struct {
	Version string `json:"Version"`
}

type apiMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

type apiPort struct {
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort"`
	Type        string `json:"Type"`
}

type apiContainer struct {
	ID     string     `json:"Id"`
	Names  []string   `json:"Names"`
	Image  string     `json:"Image"`
	State  string     `json:"State"`
	Status string     `json:"Status"`
	Ports  []apiPort  `json:"Ports"`
	Mounts []apiMount `json:"Mounts"`
}

type apiInspect struct {
	Config struct {
		Env []string `json:"Env"`
	} `json:"Config"`
}

type apiVolumes struct {
	Volumes []struct {
		Name       string `json:"Name"`
		Driver     string `json:"Driver"`
		Mountpoint string `json:"Mountpoint"`
	} `json:"Volumes"`
}

func (d *Docker) version(ctx context.Context) (string, error) {
	var v apiVersion
	if err := d.get(ctx, "/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

func (d *Docker) containers(ctx context.Context) ([]apiContainer, error) {
	var out []apiContainer
	if err := d.get(ctx, "/containers/json?all=1", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// envKeys returns the variable names of a container's environment, never the values.
func (d *Docker) envKeys(ctx context.Context, id string) ([]string, error) {
	var in apiInspect
	if err := d.get(ctx, "/containers/"+id+"/json", &in); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(in.Config.Env))
	for _, kv := range in.Config.Env {
		k, _, _ := strings.Cut(kv, "=")
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (d *Docker) volumes(ctx context.Context) (apiVolumes, error) {
	var out apiVolumes
	err := d.get(ctx, "/volumes", &out)
	return out, err
}

func (d *Docker) networkCount(ctx context.Context) (int, error) {
	var out []json.RawMessage
	if err := d.get(ctx, "/networks", &out); err != nil {
		return 0, err
	}
	return len(out), nil
}
