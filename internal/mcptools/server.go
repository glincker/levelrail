// Package mcptools builds the MCP tool server every levelrail-mcp
// transport (stdio, network, and the in-process transport an embedded
// caller like internal/ai uses) shares: one *mcp.Server with every tool
// registered against the same apiclient.Client. Pulled out of
// cmd/levelrail-mcp (which cannot be imported, being package main) so a
// non-CLI caller can build the identical tool server without spawning a
// subprocess.
package mcptools

import (
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer builds the MCP server and registers every tool against
// client. Callers can connect to it directly over an in-memory transport
// (mcp.NewInMemoryTransports), without spawning a real process or
// touching stdio.
func NewServer(client *apiclient.Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "levelrail-mcp", Version: version.Version}, nil)

	registerAppTools(server, client)
	registerDatabaseTools(server, client)
	registerServiceTemplateTools(server, client)
	registerNodeTools(server, client)
	registerPreviewTools(server, client)
	registerAlertTools(server, client)
	registerAppMetricsTools(server, client)
	registerDiagnosticTools(server, client)
	registerResourceRecommendationTools(server, client)
	registerFeatureFlagTools(server, client)
	registerSystemTools(server, client)
	registerWebhookTools(server, client)
	registerBackupVerificationTools(server, client)
	registerDeployCompareTools(server, client)
	registerPromoteTools(server, client)
	registerNotificationTools(server, client)
	registerAuditTools(server, client)
	registerIAMTools(server, client)
	registerOrganizationTools(server, client)
	registerRegistryCredentialTools(server, client)
	registerAppConfigTools(server, client)
	registerBackupTargetTools(server, client)
	registerVolumeBackupTools(server, client)
	registerScheduledTaskTools(server, client)
	registerEnvironmentTools(server, client)
	registerDomainTools(server, client)
	registerCloudflareTools(server, client)
	registerCertificateTools(server, client)
	registerLogDrainTools(server, client)
	registerSettingsTools(server, client)
	registerDeployApprovalTools(server, client)
	registerAttentionTools(server, client)
	registerControlPlaneBackupTools(server, client)
	registerFailedDeployTools(server, client)
	registerLogArchiveTools(server, client)
	registerBuildCacheTools(server, client)
	registerPipelineTools(server, client)
	registerModelTools(server, client)
	registerLoadBalancerTools(server, client)

	return server
}
