package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EmailSettingsStore is the store surface GET/PUT
// /api/v1/settings/email need: the single platform-wide row, always
// present, the same shape IngressSettingsStore has for its own row.
type EmailSettingsStore interface {
	GetEmailSettings(ctx context.Context) (store.EmailSettings, error)
	UpdateEmailSettings(ctx context.Context, s store.EmailSettings) error
	GetEmailBranding(ctx context.Context) (store.EmailBranding, error)
	SetEmailBranding(ctx context.Context, b store.EmailBranding) error
}

// CloudflareTunnelStore is the store surface GET/PUT/DELETE
// /api/v1/settings/cloudflare-tunnel need: the single platform-wide row,
// always present, the same shape EmailSettingsStore has for its own row.
type CloudflareTunnelStore interface {
	GetCloudflareTunnelSettings(ctx context.Context) (store.CloudflareTunnelSettings, error)
	UpdateCloudflareTunnelSettings(ctx context.Context, s store.CloudflareTunnelSettings) error
}

// ObservabilityStore is the store surface GET/PUT
// /api/v1/settings/observability need: the single platform-wide row,
// always present, the same shape EmailSettingsStore has for its own row.
type ObservabilityStore interface {
	GetObservabilitySettings(ctx context.Context) (store.ObservabilitySettings, error)
	UpdateObservabilitySettings(ctx context.Context, s store.ObservabilitySettings) error
}

// CloudflareDNSStore is the store surface GET/PUT
// /api/v1/settings/cloudflare-dns need, the same "single platform-wide
// row" shape CloudflareTunnelStore already establishes.
type CloudflareDNSStore interface {
	GetCloudflareDNSSettings(ctx context.Context) (store.CloudflareDNSSettings, error)
	UpdateCloudflareDNSSettings(ctx context.Context, s store.CloudflareDNSSettings) error
}

// Route53DNSStore is the store surface GET/PUT
// /api/v1/settings/route53-dns need, the same "single platform-wide
// row" shape CloudflareDNSStore already establishes for a distinct ACME
// DNS-01 provider.
type Route53DNSStore interface {
	GetRoute53DNSSettings(ctx context.Context) (store.Route53DNSSettings, error)
	UpdateRoute53DNSSettings(ctx context.Context, s store.Route53DNSSettings) error
}

// RegistryStore is the store surface GET/PUT/DELETE
// /api/v1/settings/registry need: the single platform-wide row, always
// present, the same shape CloudflareTunnelStore has for its own row.
type RegistryStore interface {
	GetRegistrySettings(ctx context.Context) (store.RegistrySettings, error)
	UpdateRegistrySettings(ctx context.Context, s store.RegistrySettings) error
}

// VaultSettingsStore is the store surface GET/PUT/DELETE
// /api/v1/settings/vault need: the single platform-wide row, always
// present, the same shape CloudflareTunnelStore has for its own row.
type VaultSettingsStore interface {
	GetVaultSettings(ctx context.Context) (store.VaultSettings, error)
	UpdateVaultSettings(ctx context.Context, s store.VaultSettings) error
	SetVaultCredentialRotation(ctx context.Context, rotatedAt, previousExpiresAt time.Time) error
}

// RegistryAuthTester asks the local Docker daemon to authenticate against a registry without
// pulling. Kept off docker.Runtime to avoid rippling into its fakes. *docker.Client satisfies it.
type RegistryAuthTester interface {
	TestRegistryAuth(ctx context.Context, host, username, password string) error
}

// GoogleCloudDNSStore is the store surface GET/PUT/DELETE
// /api/v1/settings/google-cloud-dns need.
type GoogleCloudDNSStore interface {
	GetGoogleCloudDNSSettings(ctx context.Context) (store.GoogleCloudDNSSettings, error)
	UpdateGoogleCloudDNSSettings(ctx context.Context, s store.GoogleCloudDNSSettings) error
}
