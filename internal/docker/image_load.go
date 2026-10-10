package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	dockerclient "github.com/docker/docker/client"
)

// ImageLoader loads a `docker save` tar stream into the node's image store.
type ImageLoader interface {
	LoadImage(ctx context.Context, r io.Reader) error
}

// LoadImage implements ImageLoader through the Engine API's image load
// endpoint. r is read to the end but not closed.
func (c *Client) LoadImage(ctx context.Context, r io.Reader) error {
	resp, err := c.cli.ImageLoad(ctx, r, dockerclient.ImageLoadWithQuiet(true))
	if err != nil {
		return fmt.Errorf("docker: load image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return readLoadResponse(resp.Body, resp.JSON)
}

func readLoadResponse(body io.Reader, isJSON bool) error {
	if !isJSON {
		if _, err := io.Copy(io.Discard, body); err != nil {
			return fmt.Errorf("docker: drain image load response: %w", err)
		}
		return nil
	}
	dec := json.NewDecoder(body)
	for {
		var msg struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("docker: read image load response: %w", err)
		}
		if msg.Error != "" {
			return fmt.Errorf("docker: load image: %s", msg.Error)
		}
	}
}
