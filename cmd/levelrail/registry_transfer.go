package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	registryreconcile "github.com/GLINCKER/levelrail/internal/reconcile/registry"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvRegistryTokenKeyFile points at the P-256 key (PEM) that signs registry bearer tokens.
const EnvRegistryTokenKeyFile = "APP_REGISTRY_TOKEN_KEY_FILE" //nolint:gosec // env var name, not a credential

// registryTransfer is built once by the builder and shared with the API token route.
type registryTransfer struct {
	issuer *build.PushTokenIssuer
	tokens *build.RegistryTokenService
	host   atomic.Value
	// ttl holds the live bearer TTL reader; settings lets the reconciler read build.transfer_mode itself.
	ttl      atomic.Value
	settings atomic.Pointer[runtimesettings.Service]
}

var (
	sharedTransferMu sync.Mutex
	sharedTransfer   *registryTransfer
)

func sharedRegistryTransfer() *registryTransfer {
	sharedTransferMu.Lock()
	defer sharedTransferMu.Unlock()
	return sharedTransfer
}

// newRegistryTransfer returns nil when no signing key exists, which keeps every build on tar.
func newRegistryTransfer(logger *slog.Logger) *registryTransfer {
	path := os.Getenv(EnvRegistryTokenKeyFile)
	if path == "" {
		return nil
	}
	pemBytes, err := os.ReadFile(path) //nolint:gosec // operator-set path, not request input
	if err != nil {
		logger.Warn("registry transfer disabled: signing key unreadable")
		return nil
	}
	der, err := decodeECPEM(pemBytes)
	if err != nil {
		logger.Warn("registry transfer disabled: signing key is not valid")
		return nil
	}
	key, err := build.ParseECKeyPEM(der)
	if err != nil {
		logger.Warn("registry transfer disabled: signing key is not valid")
		return nil
	}
	// The push token key lives only in memory: a restart ends every build anyway.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil
	}
	issuer, err := build.NewPushTokenIssuer(secret)
	if err != nil {
		return nil
	}
	t := &registryTransfer{issuer: issuer}
	t.tokens = &build.RegistryTokenService{
		Issuer: issuer, Key: key, Service: registryTokenService, TokenIssuer: registryTokenIssuer,
		BearerTTL: func() time.Duration {
			return t.bearerTTL()
		},
		Host: func() string {
			h, _ := t.host.Load().(string)
			return h
		},
	}
	t.installTokenAuth(key, logger)
	sharedTransferMu.Lock()
	sharedTransfer = t
	sharedTransferMu.Unlock()
	return t
}

//nolint:gosec // public service names and a URL path, not credentials
const (
	registryTokenService = "levelrail-registry"
	registryTokenIssuer  = "levelrail"
	registryTokenPath    = "/api/v1/registry/token"
)

// bearerTTL reads the live setting; before settings are attached it is zero, which fails closed.
func (t *registryTransfer) bearerTTL() time.Duration {
	if f, ok := t.ttl.Load().(func() time.Duration); ok {
		return f()
	}
	return 0
}

// installTokenAuth lets the registry reconciler switch to token auth, but only while
// transfer_mode is registry and a realm URL is known; otherwise htpasswd stays.
func (t *registryTransfer) installTokenAuth(key *ecdsa.PrivateKey, logger *slog.Logger) {
	bundle, err := build.CertBundlePEM(key, time.Now(), registryCertValidity)
	if err != nil {
		logger.Warn("registry transfer disabled: signing certificate could not be built")
		return
	}
	registryreconcile.SetTokenAuthSource(func(ctx context.Context) *registryreconcile.TokenAuth {
		st := t.settings.Load()
		if st == nil {
			return nil
		}
		if m, err := build.ParseTransferMode(st.String(ctx, runtimesettings.KeyBuildTransferMode)); err != nil || m != build.TransferRegistry {
			return nil
		}
		base := dashboardBaseURL()
		if base == "" {
			return nil
		}
		return &registryreconcile.TokenAuth{Realm: base + registryTokenPath, Service: registryTokenService, Issuer: registryTokenIssuer, CertBundlePEM: bundle}
	})
}

// registryCertValidity is long because the certificate only carries the public key.
const registryCertValidity = 10 * 365 * 24 * time.Hour

// config is read on every build so build.transfer_mode applies live.
func (t *registryTransfer) config(db *store.DB, settings *runtimesettings.Service, cli *dockerclient.Client, logger *slog.Logger) func(ctx context.Context) build.TransferConfig {
	if t != nil {
		t.settings.Store(settings)
		t.ttl.Store(func() time.Duration {
			return settings.Duration(context.Background(), runtimesettings.KeyBuildTransferBearerTTL)
		})
	}
	return func(ctx context.Context) build.TransferConfig {
		mode, err := build.ParseTransferMode(settings.String(ctx, runtimesettings.KeyBuildTransferMode))
		if err != nil || mode != build.TransferRegistry || t == nil {
			return build.TransferConfig{Mode: build.TransferTar}
		}
		rs, err := db.GetRegistrySettings(ctx)
		if err != nil || !rs.Enabled || rs.Host == "" {
			return build.TransferConfig{Mode: build.TransferTar}
		}
		insecure := strings.EqualFold(os.Getenv("APP_BUILD_TRANSFER_INSECURE"), "true")
		if insecure {
			allowCGNAT := strings.EqualFold(os.Getenv("APP_BUILD_TRANSFER_ALLOW_CGNAT"), "true")
			if err := build.ValidateInsecureHost(rs.Host, allowCGNAT); err != nil {
				logger.Warn("registry transfer disabled: insecure registry host is not private")
				return build.TransferConfig{Mode: build.TransferTar}
			}
			logger.Warn("registry transfer uses plain HTTP to a private registry host")
		}
		t.host.Store(rs.Host)
		return build.TransferConfig{
			Mode: mode, RegistryHost: rs.Host, Insecure: insecure, TokenAuth: registryreconcile.TokenAuthActive(),
			TokenTTL: settings.Duration(ctx, runtimesettings.KeyBuildTransferPushTTL),
			Issuer:   t.issuer,
			Verifier: &build.RegistryClient{Insecure: insecure, Bearer: t.tokens.MintPull},
			Pull:     t.pull(cli),
		}
	}
}

// pull fetches repo@digest and tags it locally; the registry tag is never consumed.
func (t *registryTransfer) pull(cli *dockerclient.Client) func(ctx context.Context, ref, digest, tag string) error {
	return func(ctx context.Context, ref, digest, tag string) error {
		repo, _, err := build.SplitRef(ref)
		if err != nil {
			return err
		}
		_, name, _ := strings.Cut(repo, "/")
		tok, err := t.tokens.MintPull(name)
		if err != nil {
			return err
		}
		auth, err := registry.EncodeAuthConfig(registry.AuthConfig{RegistryToken: tok})
		if err != nil {
			return errors.New("registry transfer: encode pull auth")
		}
		byDigest := repo + "@" + digest
		rc, err := cli.ImagePull(ctx, byDigest, image.PullOptions{RegistryAuth: auth})
		if err != nil {
			return fmt.Errorf("registry transfer: pull by digest: %w", err)
		}
		_, copyErr := io.Copy(io.Discard, rc)
		_ = rc.Close()
		if copyErr != nil {
			return fmt.Errorf("registry transfer: pull stream: %w", copyErr)
		}
		insp, err := cli.ImageInspect(ctx, byDigest)
		if err != nil {
			return fmt.Errorf("registry transfer: inspect pulled image: %w", err)
		}
		found := false
		for _, rd := range insp.RepoDigests {
			if rd == byDigest {
				found = true
			}
		}
		if !found {
			return errors.New("registry transfer: pulled image does not carry the expected digest")
		}
		return cli.ImageTag(ctx, byDigest, tag)
	}
}

func decodeECPEM(b []byte) ([]byte, error) {
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, errors.New("no PEM block")
	}
	return blk.Bytes, nil
}
