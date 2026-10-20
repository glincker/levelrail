package platformimport

import (
	"context"
	"errors"
	"fmt"
)

// InspectReader returns the inspect JSON of every container on a host as one array.
type InspectReader interface {
	InspectAll(ctx context.Context) ([]byte, error)
}

// DockerSSHSource reads a host's containers through a read-only Docker
// socket tunnel and maps them like a pasted docker inspect snapshot. Env
// comes from each container's runtime config; the old platform's own stored
// config (Coolify's database, CapRover's /captain/data, Dokku's
// /home/dokku) is not read.
type DockerSSHSource struct {
	Reader InspectReader
}

// Discover reads every container over the tunnel and parses the result.
func (s DockerSSHSource) Discover(ctx context.Context) (*Discovery, error) {
	if s.Reader == nil {
		return nil, errors.New("no docker connection")
	}
	raw, err := s.Reader.InspectAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("read containers over ssh: %w", err)
	}
	d, err := DockerSource{Data: raw}.Discover(ctx)
	if err != nil {
		return nil, err
	}
	d.Platform = DockerSSH
	for i := range d.Apps {
		d.Apps[i].Notes = append(d.Apps[i].Notes, Note{
			Reason: "env and settings come from the running container; the old platform's stored config was not read",
			Manual: "check domains, build settings and variables that are not set on the container before cutover",
		})
	}
	return d, nil
}
