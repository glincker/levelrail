package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// maxTailBytes bounds what one TailLogs call reads, so a runaway log line cannot bloat a failure record.
const maxTailBytes = 64 * 1024

var errTailFull = errors.New("log tail cap reached")

// TailLogs returns the last lines of a container's combined output through the Engine API, without timestamps.
func (c *Client) TailLogs(ctx context.Context, name string, lines int) (string, error) {
	rc, err := c.cli.ContainerLogs(ctx, name, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(lines),
	})
	if err != nil {
		return "", fmt.Errorf("docker: tail logs %q: %w", name, err)
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	w := &cappedWriter{w: &buf, left: maxTailBytes}
	if _, err := stdcopy.StdCopy(w, w, rc); err != nil && !errors.Is(err, errTailFull) {
		return buf.String(), fmt.Errorf("docker: read logs %q: %w", name, err)
	}
	return buf.String(), nil
}

type cappedWriter struct {
	w    *bytes.Buffer
	left int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	full := len(p) > c.left
	if full {
		p = p[:c.left]
	}
	c.left -= len(p)
	n, err := c.w.Write(p)
	if full {
		return n, errTailFull
	}
	return n, err
}
