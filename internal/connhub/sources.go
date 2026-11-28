package connhub

import (
	"context"

	"github.com/glincker/levelrail/internal/store"
)

// NotifyChannel is the slice of a notification channel the hub shows, kept here so this package
// does not import the alerting package and its large dependency tree.
type NotifyChannel struct {
	ID        string
	Name      string
	Kind      string
	Enabled   bool
	CreatedAt string
}

// The narrow readers below are satisfied by *store.DB (notify goes through an adapter in the api layer), so tests use
// small fakes. A nil source is skipped: its kind simply lists nothing.
type (
	// GitReader reads forge connections and their recorded health.
	GitReader interface {
		ListGitConnections(ctx context.Context, provider string) ([]store.GitConnection, error)
		GetGitConnectionStatus(ctx context.Context, provider string) (store.GitConnectionStatus, bool, error)
		GetGitHubAppConnection(ctx context.Context) (store.GitHubAppConnection, error)
		ListGitHubAppInstallations(ctx context.Context) ([]store.GitHubAppInstallation, error)
		ListGitSourcesByConnection(ctx context.Context, connectionID string) ([]string, error)
		GetGitSource(ctx context.Context, serviceName string) (*store.GitSource, error)
	}
	// RegistryReader reads registry credentials.
	RegistryReader interface {
		ListRegistryCredentials(ctx context.Context) ([]store.RegistryCredential, error)
	}
	// NotifyReader reads notification channels.
	NotifyReader interface {
		ListNotificationChannels(ctx context.Context) ([]NotifyChannel, error)
	}
	// SecretReader reads named secret provider connections.
	SecretReader interface {
		ListSecretConnections(ctx context.Context) ([]store.SecretConnection, error)
	}
	// DNSReader reads the per provider DNS settings rows.
	DNSReader interface {
		GetCloudflareDNSSettings(ctx context.Context) (store.CloudflareDNSSettings, error)
		GetRoute53DNSSettings(ctx context.Context) (store.Route53DNSSettings, error)
		GetGoogleCloudDNSSettings(ctx context.Context) (store.GoogleCloudDNSSettings, error)
	}
	// CloudReader reads cloud account connections.
	CloudReader interface {
		ListCloudConnections(ctx context.Context) ([]store.CloudConnection, error)
	}
	// WebhookReader reads outbound event and status page webhooks.
	WebhookReader interface {
		ListEventWebhooks(ctx context.Context) ([]store.EventWebhook, error)
		ListStatusWebhooks(ctx context.Context) ([]store.StatusWebhook, error)
	}
	// MetaReader reads the hub sidecar and bindings tables.
	MetaReader interface {
		ListConnectionMeta(ctx context.Context) (map[string]store.ConnectionMeta, error)
		CountConnectionBindingsByRef(ctx context.Context) (map[string]int, error)
		ListConnectionBindingsByTarget(ctx context.Context, targetType, targetID string) ([]store.ConnectionBinding, error)
	}
)

// Sources bundles the readers the service projects from.
type Sources struct {
	Git      GitReader
	Registry RegistryReader
	Notify   NotifyReader
	Secrets  SecretReader
	DNS      DNSReader
	Cloud    CloudReader
	Webhooks WebhookReader
	Meta     MetaReader
}
