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
	server, _ := NewServerWithOptions(client, Options{Mode: ModeFull})
	return server
}

// NewServerWithOptions builds the MCP server and then unregisters every
// tool the mode and toolsets exclude, so a hidden tool costs no model
// context and cannot be called.
func NewServerWithOptions(client *apiclient.Client, opts Options) (*mcp.Server, Summary) {
	server := mcp.NewServer(&mcp.Implementation{Name: "levelrail-mcp", Version: version.Version},
		&mcp.ServerOptions{CompletionHandler: newCompletionHandler(client)})

	registerAppTools(server, client)
	registerDeployFreezeTools(server, client)
	registerAppScheduleTools(server, client)
	registerDeployCancelTools(server, client)
	registerAppTimelineTools(server, client)
	registerBulkAppTools(server, client)
	registerDatabaseTools(server, client)
	registerDatabaseAccessTools(server, client)
	registerServiceTemplateTools(server, client)
	registerNodeTools(server, client)
	registerPluginTools(server, client)
	registerPreviewTools(server, client)
	registerAlertTools(server, client)
	registerAlertNoiseTools(server, client)
	registerDashboardTools(server, client)
	registerResilienceTools(server, client)
	registerArchiveTools(server, client)
	registerEncryptionKeyTools(server, client)
	registerAppMetricsTools(server, client)
	registerAppRequestsTool(server, client)
	registerAppTrafficTool(server, client)
	registerDiagnosticTools(server, client)
	registerResourceRecommendationTools(server, client)
	registerResourceAdviceTools(server, client)
	registerUsageReportTools(server, client)
	registerCostExplorerTools(server, client)
	registerFeatureFlagTools(server, client)
	registerSystemTools(server, client)
	registerReleaseTools(server, client)
	registerWebhookTools(server, client)
	registerBackupVerificationTools(server, client)
	registerDatabasePITRTools(server, client)
	registerDeployCompareTools(server, client)
	registerPromoteTools(server, client)
	registerNotificationTools(server, client)
	registerAuditTools(server, client)
	registerAccountTools(server, client)
	registerIAMTools(server, client)
	registerRoleTools(server, client)
	registerOrganizationTools(server, client)
	registerRegistryCredentialTools(server, client)
	registerAppConfigTools(server, client)
	registerCanaryTools(server, client)
	registerEnvTools(server, client)
	registerGetEnvTool(server, client)
	registerBackupTargetTools(server, client)
	registerVolumeBackupTools(server, client)
	registerScheduledTaskTools(server, client)
	registerEnvironmentTools(server, client)
	registerGlobalEnvironmentTools(server, client)
	registerDomainTools(server, client)
	registerImportTools(server, client)
	registerMigrationHubTools(server, client)
	registerAppImportTools(server, client)
	registerAppImportExitTool(server, client)
	registerCloudflareTools(server, client)
	registerCertificateTools(server, client)
	registerLogDrainTools(server, client)
	registerSettingsTools(server, client)
	registerDeployApprovalTools(server, client)
	registerAttentionTools(server, client)
	registerControlPlaneBackupTools(server, client)
	registerControlPlaneDRTools(server, client)
	registerFailedDeployTools(server, client)
	registerDeploymentTools(server, client)
	registerWaitDeployTool(server, client)
	registerPlanTools(server, client)
	registerLogArchiveTools(server, client)
	registerQueryLogsTool(server, client)
	registerLogSignaturesTool(server, client)
	registerBuildCacheTools(server, client)
	registerJanitorTools(server, client)
	registerSearchTools(server, client)
	registerNewSurfaceTools(server, client)
	registerBuildTools(server, client)
	registerPipelineTools(server, client)
	registerModelTools(server, client)
	registerModelServingTools(server, client)
	registerLoadBalancerTools(server, client)
	registerIaCTools(server, client)
	registerAIControlTools(server, client)
	registerExpansionReadTools(server, client)
	registerExpansionWriteTools(server, client)
	registerInterfaceExtraTools(server, client)
	registerSecurityTools(server, client)
	registerMigrateTools(server, client)
	registerIdentityTools(server, client)

	registerPrompts(server, opts)
	registerResources(server, client)

	opts.probe = &abilityProbe{client: client}
	summary := applyOptions(server, client, opts)
	server.AddReceivingMiddleware(aiControlMiddleware(client), abilityMiddleware(opts.probe), fieldsProjection)
	return server, summary
}
