package docker

import (
	"context"
	"fmt"
	"strconv"
)

// LogRotationInfo is the log driver setup a container was created with.
type LogRotationInfo struct {
	Driver  string
	MaxSize string
	MaxFile int
}

// LogRotationInspector is an optional Runtime capability (type-asserted) read by the doctor.
type LogRotationInspector interface {
	InspectLogRotation(ctx context.Context, id string) (LogRotationInfo, error)
}

// InspectLogRotation implements LogRotationInspector with one container inspect.
func (c *Client) InspectLogRotation(ctx context.Context, id string) (LogRotationInfo, error) {
	resp, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return LogRotationInfo{}, fmt.Errorf("docker: inspect container %q: %w", id, err)
	}
	if resp.HostConfig == nil {
		return LogRotationInfo{}, nil
	}
	lc := resp.HostConfig.LogConfig
	n, _ := strconv.Atoi(lc.Config["max-file"])
	return LogRotationInfo{Driver: lc.Type, MaxSize: lc.Config["max-size"], MaxFile: n}, nil
}
