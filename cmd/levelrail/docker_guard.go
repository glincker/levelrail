package main

import (
	"context"
	"fmt"
	"log/slog"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dockerGuardSink writes each guard decision as an audit_log row.
type dockerGuardSink struct{ db *store.DB }

func (s dockerGuardSink) RecordDecision(ctx context.Context, d dockerguard.Decision) error {
	e, err := api.DockerGuardAuditEntry(d)
	if err != nil {
		return fmt.Errorf("docker guard audit entry: %w", err)
	}
	return s.db.SaveAuditEntry(ctx, e)
}

// bootDockerGuard starts the Docker API guard before any Docker client
// exists, so every client below dials through it. Enforce-mode failures
// stop the control plane rather than run it unguarded.
func bootDockerGuard(ctx context.Context, logger *slog.Logger, db *store.DB) (dockerguard.Booted, error) {
	b, err := dockerguard.Boot(ctx, dockerguard.BootConfig{
		DataDir:         dataDirFromEnv(),
		ResolveSymlinks: !dockerguard.RunningInContainer(),
		Sink:            dockerGuardSink{db: db},
		Logger:          logger,
	})
	if err != nil {
		return dockerguard.Booted{}, fmt.Errorf("docker guard: %w", err)
	}
	return b, nil
}

// guardClientOptions routes an internal/docker Client through the guard.
func guardClientOptions(b dockerguard.Booted) []docker.ClientOption {
	opts := []docker.ClientOption{docker.WithCreateDeclarer(b.Grants)}
	if b.Host != "" {
		opts = append(opts, docker.WithHost(b.Host))
	}
	return opts
}

// rawDockerClient is the SDK client BuildKit needs, through the guard
// when it is running.
func rawDockerClient(guardHost string) (*dockerclient.Client, error) {
	opts := []dockerclient.Opt{dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation()}
	if guardHost != "" {
		opts = append(opts, dockerclient.WithHost(guardHost))
	}
	return dockerclient.NewClientWithOpts(opts...)
}
