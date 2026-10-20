package dockerssh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	maxListBody       = 16 << 20
	maxInspectedCount = 2000
)

// Overview is what a connection test reports it can read.
type Overview struct {
	DockerVersion string `json:"docker_version"`
	OS            string `json:"os,omitempty"`
	Swarm         bool   `json:"swarm"`
	Containers    int    `json:"containers"`
	Running       int    `json:"running"`
	Volumes       int    `json:"volumes"`
	Images        int    `json:"images"`
}

func (s *Session) get(ctx context.Context, p string, query url.Values) (*http.Response, error) {
	u := url.URL{Scheme: "http", Host: dockerPseudoHost, Path: p, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build docker request: %w", err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("docker answered %d for GET %s: %s", resp.StatusCode, p, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (s *Session) getJSON(ctx context.Context, p string, query url.Values) ([]byte, error) {
	resp, err := s.get(ctx, p, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxListBody+1))
	if err != nil {
		return nil, fmt.Errorf("read GET %s: %w", p, err)
	}
	if len(b) > maxListBody {
		return nil, fmt.Errorf("the answer to GET %s is too large", p)
	}
	return b, nil
}

// Ping checks that the daemon answers on the socket.
func (s *Session) Ping(ctx context.Context) error {
	resp, err := s.get(ctx, "/_ping", nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

type containerSummary struct {
	ID    string `json:"Id"`
	State string `json:"State"`
}

func (s *Session) listContainers(ctx context.Context) ([]containerSummary, error) {
	raw, err := s.getJSON(ctx, "/containers/json", url.Values{"all": {"1"}})
	if err != nil {
		return nil, err
	}
	var list []containerSummary
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parse the container list: %w", err)
	}
	if len(list) > maxInspectedCount {
		return nil, fmt.Errorf("the host has %d containers, more than the %d this import reads", len(list), maxInspectedCount)
	}
	return list, nil
}

func countJSON(raw []byte, key string) int {
	if key == "" {
		var l []json.RawMessage
		if json.Unmarshal(raw, &l) != nil {
			return 0
		}
		return len(l)
	}
	var m map[string][]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return 0
	}
	return len(m[key])
}

// Probe reads the version and counts what an import can see.
func (s *Session) Probe(ctx context.Context) (Overview, error) {
	var o Overview
	raw, err := s.getJSON(ctx, "/version", nil)
	if err != nil {
		return o, err
	}
	var v struct {
		Version string `json:"Version"`
		OS      string `json:"Os"`
	}
	_ = json.Unmarshal(raw, &v)
	o.DockerVersion, o.OS = v.Version, v.OS
	if raw, err = s.getJSON(ctx, "/info", nil); err == nil {
		var info struct {
			Swarm struct {
				LocalNodeState string `json:"LocalNodeState"`
			} `json:"Swarm"`
		}
		_ = json.Unmarshal(raw, &info)
		o.Swarm = info.Swarm.LocalNodeState == "active"
	}
	cs, err := s.listContainers(ctx)
	if err != nil {
		return o, err
	}
	o.Containers = len(cs)
	for _, c := range cs {
		if c.State == "running" {
			o.Running++
		}
	}
	if raw, err = s.getJSON(ctx, "/volumes", nil); err == nil {
		o.Volumes = countJSON(raw, "Volumes")
	}
	if raw, err = s.getJSON(ctx, "/images/json", nil); err == nil {
		o.Images = countJSON(raw, "")
	}
	return o, nil
}

// InspectAll returns the inspect JSON of every container as one array, the
// same shape `docker inspect` prints.
func (s *Session) InspectAll(ctx context.Context) ([]byte, error) {
	cs, err := s.listContainers(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, c := range cs {
		raw, err := s.getJSON(ctx, "/containers/"+c.ID+"/json", nil)
		if err != nil {
			return nil, fmt.Errorf("inspect container %.12s: %w", c.ID, err)
		}
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(bytes.TrimSpace(raw))
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// ImageSave streams the image as `docker save` would, through the tunnel.
func (s *Session) ImageSave(ctx context.Context, ref string) (io.ReadCloser, error) {
	resp, err := s.get(ctx, "/images/"+ref+"/get", nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ContainerArchive streams a tar of one of the container's own mount
// destinations. The container is inspected first so the allowlist learns
// which destinations it may ask for.
func (s *Session) ContainerArchive(ctx context.Context, containerID, mountPath string) (io.ReadCloser, error) {
	if _, err := s.getJSON(ctx, "/containers/"+containerID+"/json", nil); err != nil {
		return nil, fmt.Errorf("inspect container %.12s: %w", containerID, err)
	}
	resp, err := s.get(ctx, "/containers/"+containerID+"/archive", url.Values{"path": {mountPath}})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// SwarmServices lists swarm services on a manager node, for Swarm sources.
func (s *Session) SwarmServices(ctx context.Context) ([]byte, error) {
	return s.getJSON(ctx, "/services", nil)
}
