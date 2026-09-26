package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultPreviousReleaseHold  = 3 * time.Minute
	defaultHeldReleaseInterval  = 30 * time.Second
	previousReleaseHoldEnv      = "APP_DEPLOY_PREVIOUS_RELEASE_HOLD"
	heldDeployReleaseIntervalEv = "APP_DEPLOY_HELD_RELEASE_INTERVAL"
)

// envDurationOr parses name as a duration, falling back to def when unset
// or invalid. "0" is a valid value.
func envDurationOr(logger *slog.Logger, name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		logger.Warn("invalid duration, using default", slog.String("env", name), slog.String("value", raw), slog.String("default", def.String()))
		return def
	}
	return d
}

// previousReleaseHold is how long the application controller keeps the
// previous release running after a cutover; 0 disables the hold.
func previousReleaseHold(logger *slog.Logger) time.Duration {
	return envDurationOr(logger, previousReleaseHoldEnv, defaultPreviousReleaseHold)
}

// startHeldDeployReleaser replays deploys held by freeze windows once the
// window ends.
func startHeldDeployReleaser(ctx context.Context, logger *slog.Logger, db *store.DB, rt *api.Router) {
	interval := envDurationOr(logger, heldDeployReleaseIntervalEv, defaultHeldReleaseInterval)
	if interval <= 0 {
		interval = defaultHeldReleaseInterval
	}
	r := deploy.Releaser{Store: db, Freeze: db, Handlers: rt.ReleaseHandlers(), Logger: logger}
	go r.Run(ctx, interval)
}

// digestResolverOptions gives the build pipeline a docker client to resolve
// image tags to digests and read built image IDs. A client failure only
// disables resolution: deploys still work, unpinned.
func digestResolverOptions(logger *slog.Logger, db *store.DB, secretsManager *secrets.Manager) []deploy.Option {
	client, err := docker.NewClient()
	if err != nil {
		logger.Warn("digest resolution disabled: docker client unavailable", slog.String("error", err.Error()))
		return nil
	}
	opts := []deploy.Option{deploy.WithImageResolver(client), deploy.WithImageInspector(client)}
	if secretsManager != nil {
		opts = append(opts, deploy.WithRegistryAuthSource(registryAuthSource{db: db, secrets: secretsManager}))
	}
	return opts
}

type registryAuthSource struct {
	db      *store.DB
	secrets *secrets.Manager
}

func (s registryAuthSource) RegistryAuth(ctx context.Context, credentialID string) (*docker.RegistryAuth, error) {
	cred, err := s.db.GetRegistryCredential(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("get registry credential: %w", err)
	}
	password, err := s.secrets.Resolve(ctx, store.RegistryCredentialSecretsKey(credentialID), "password")
	if err != nil {
		return nil, fmt.Errorf("resolve registry credential password: %w", err)
	}
	return &docker.RegistryAuth{Username: cred.Username, Password: password}, nil
}
