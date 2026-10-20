package main

import (
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/version"
)

// Client and the wire types are aliases of internal/apiclient; every call dispatches there.
type (
	Client                           = apiclient.Client
	appResource                      = apiclient.AppResource
	deployTriggerResult              = apiclient.DeployTriggerResult
	deployApprovalResource           = apiclient.DeployApprovalResource
	deployApprovalDecisionResult     = apiclient.DeployApprovalDecisionResult
	serviceResources                 = apiclient.ServiceResources
	serviceProbe                     = apiclient.ServiceProbe
	serviceHealth                    = apiclient.ServiceHealth
	appHealthResource                = apiclient.AppHealthResource
	appHealthScoreResource           = apiclient.AppHealthScoreResource
	appHealthScoreCategory           = apiclient.AppHealthScoreCategory
	healthDiscoveryResponse          = apiclient.HealthDiscoveryResponse
	healthDiscoveryAttempt           = apiclient.HealthDiscoveryAttempt
	serviceHooks                     = apiclient.ServiceHooks
	buildTriggerRequest              = apiclient.BuildTriggerRequest
	buildTriggerRequestBuild         = apiclient.BuildTriggerRequestBuild
	buildTriggerResponse             = apiclient.BuildTriggerResponse
	conditionResource                = apiclient.ConditionResource
	deployStepEvent                  = apiclient.DeployStepEvent
	failedDeployResource             = apiclient.FailedDeployResource
	deployAttemptResource            = apiclient.DeployAttemptResource
	networkResource                  = apiclient.NetworkResource
	logEntryResource                 = apiclient.LogEntryResource
	logStreamEntry                   = apiclient.LogStreamEntry
	appMetricsResource               = apiclient.AppMetricsResource
	appResourceUsageResource         = apiclient.AppResourceUsageResource
	appMetricsSummary                = apiclient.AppMetricsSummary
	metricPointResource              = apiclient.MetricPointResource
	nodeMetricsResource              = apiclient.NodeMetricsResource
	execRequest                      = apiclient.ExecRequest
	execResponse                     = apiclient.ExecResponse
	domainResource                   = apiclient.DomainResource
	cloudflareDNSResource            = apiclient.CloudflareDNSResource
	updateCloudflareDNSRequest       = apiclient.UpdateCloudflareDNSRequest
	route53DNSResource               = apiclient.Route53DNSResource
	updateRoute53DNSRequest          = apiclient.UpdateRoute53DNSRequest
	cloudflareTunnelResource         = apiclient.CloudflareTunnelResource
	updateCloudflareTunnelRequest    = apiclient.UpdateCloudflareTunnelRequest
	vaultSettingsResource            = apiclient.VaultSettingsResource
	updateVaultSettingsRequest       = apiclient.UpdateVaultSettingsRequest
	appVaultEnvRef                   = apiclient.AppVaultEnvRef
	appEgressAllow                   = apiclient.AppEgressAllow
	appEgressPolicyResource          = apiclient.AppEgressPolicyResource
	setAppEgressPolicyRequest        = apiclient.SetAppEgressPolicyRequest
	appPreviewEnvOverride            = apiclient.AppPreviewEnvOverride
	appBranchEnvOverride             = apiclient.AppBranchEnvOverride
	prPreviewEnvOverride             = apiclient.PRPreviewEnvOverride
	registrySettingsResource         = apiclient.RegistrySettingsResource
	updateRegistrySettingsRequest    = apiclient.UpdateRegistrySettingsRequest
	registryRepositoriesResource     = apiclient.RegistryRepositoriesResource
	registryTagsResource             = apiclient.RegistryTagsResource
	domainBasicAuthResource          = apiclient.DomainBasicAuthResource
	setDomainBasicAuthRequest        = apiclient.SetDomainBasicAuthRequest
	domainMaintenanceResource        = apiclient.DomainMaintenanceResource
	domainTLSCertResource            = apiclient.DomainTLSCertResource
	setDomainTLSCertRequest          = apiclient.SetDomainTLSCertRequest
	domainWAFResource                = apiclient.DomainWAFResource
	setDomainWAFRequest              = apiclient.SetDomainWAFRequest
	domainAuthGateResource           = apiclient.DomainAuthGateResource
	setDomainAuthGateRequest         = apiclient.SetDomainAuthGateRequest
	authGateRule                     = apiclient.AuthGateRule
	authGateEventsResponse           = apiclient.AuthGateEventsResponse
	apiAuthGateEvent                 = apiclient.AuthGateEvent
	domainRedirectResource           = apiclient.DomainRedirectResource
	setDomainRedirectRequest         = apiclient.SetDomainRedirectRequest
	domainErrorPagesResource         = apiclient.DomainErrorPagesResource
	domainErrorPageEntry             = apiclient.DomainErrorPageEntry
	setDomainErrorPageRequest        = apiclient.SetDomainErrorPageRequest
	domainCheckResource              = apiclient.DomainCheckResource
	cloneAppRequest                  = apiclient.CloneAppRequest
	imageResource                    = apiclient.ImageResource
	backupHistoryResource            = apiclient.BackupHistoryResource
	backupVerificationResource       = apiclient.BackupVerificationResource
	restoreHistoryResource           = apiclient.RestoreHistoryResource
	sessionInfoResource              = apiclient.SessionInfoResource
	whoamiResource                   = apiclient.WhoamiResource
	sessionLinkResource              = apiclient.SessionLinkResource
	databaseResource                 = apiclient.DatabaseResource
	databaseEngineResource           = apiclient.DatabaseEngineResource
	setDatabaseResourcesRequest      = apiclient.SetDatabaseResourcesRequest
	databasePublicAccessResource     = apiclient.DatabasePublicAccessResource
	setDatabasePublicAccessRequest   = apiclient.SetDatabasePublicAccessRequest
	slowQueryEntryResource           = apiclient.SlowQueryEntryResource
	composeDeployResult              = apiclient.ComposeDeployResult
	apiError                         = apiclient.APIError
	deployTriggerRequest             = apiclient.DeployTriggerRequest
	triggerBackupRequest             = apiclient.TriggerBackupRequest
	backupScheduleResource           = apiclient.BackupScheduleResource
	setBackupScheduleRequest         = apiclient.SetBackupScheduleRequest
	triggerRestoreRequest            = apiclient.TriggerRestoreRequest
	pitrStatusResource               = apiclient.PITRStatusResource
	baseBackupHistoryResource        = apiclient.BaseBackupHistoryResource
	triggerBaseBackupRequest         = apiclient.TriggerBaseBackupRequest
	pitrRestoreHistoryResource       = apiclient.PITRRestoreHistoryResource
	triggerPITRRestoreRequest        = apiclient.TriggerPITRRestoreRequest
	appVolumeResource                = apiclient.AppVolumeResource
	setAppVolumesRequest             = apiclient.SetAppVolumesRequest
	setAppVolumesResponse            = apiclient.SetAppVolumesResponse
	appVolumeMoveResource            = apiclient.AppVolumeMoveResource
	appVolumeMoveStepResource        = apiclient.AppVolumeMoveStepResource
	appBindMountResource             = apiclient.AppBindMountResource
	volumeBackupScheduleResource     = apiclient.VolumeBackupScheduleResource
	setVolumeBackupScheduleRequest   = apiclient.SetVolumeBackupScheduleRequest
	cloneRestoreResource             = apiclient.CloneRestoreResource
	triggerCloneRestoreRequest       = apiclient.TriggerCloneRestoreRequest
	volumeCloneRestoreResource       = apiclient.VolumeCloneRestoreResource
	triggerVolumeCloneRestoreRequest = apiclient.TriggerVolumeCloneRestoreRequest
	setAppDatabaseRequest            = apiclient.SetAppDatabaseRequest
	appDatabaseResource              = apiclient.AppDatabaseResource
	createAppConnectionRequest       = apiclient.CreateAppConnectionRequest
	appConnectionResource            = apiclient.AppConnectionResource
	connectableDatabaseResource      = apiclient.ConnectableDatabaseResource
	appStatusSummary                 = apiclient.AppStatusSummary
	appGroupResource                 = apiclient.AppGroupResource
	hookRunResource                  = apiclient.HookRunResource
	appHookRunsResource              = apiclient.AppHookRunsResource
	deploySpecServiceBuild           = apiclient.DeploySpecServiceBuild
	deploySpecServiceEnv             = apiclient.DeploySpecServiceEnv
	deploySpecService                = apiclient.DeploySpecService
	deploySpecServiceHooks           = apiclient.DeploySpecServiceHooks
	deploySpecRequest                = apiclient.DeploySpecRequest
	deploySpecServiceResult          = apiclient.DeploySpecServiceResult
	deploySpecResult                 = apiclient.DeploySpecResult
	diagnosisResource                = apiclient.DiagnosisResource
	diagnosisSignal                  = apiclient.DiagnosisSignal
	resourceRecommendationResource   = apiclient.ResourceRecommendationResource
	dimensionRecommendationResource  = apiclient.DimensionRecommendationResource
	costEstimateResource             = apiclient.CostEstimateResource
	costEstimateProviderResource     = apiclient.CostEstimateProviderResource

	backupTargetResource            = apiclient.BackupTargetResource
	createBackupTargetRequest       = apiclient.CreateBackupTargetRequest
	updateBackupTargetRequest       = apiclient.UpdateBackupTargetRequest
	registryCredentialResource      = apiclient.RegistryCredentialResource
	createRegistryCredentialRequest = apiclient.CreateRegistryCredentialRequest
	updateRegistryCredentialRequest = apiclient.UpdateRegistryCredentialRequest
	networkShareResource            = apiclient.NetworkShareResource
	createNetworkShareRequest       = apiclient.CreateNetworkShareRequest
	updateNetworkShareRequest       = apiclient.UpdateNetworkShareRequest
	networkTopologyResource         = apiclient.NetworkTopologyResource
	networkTopologyNodeResource     = apiclient.NetworkTopologyNodeResource
	networkTopologyAppResource      = apiclient.NetworkTopologyAppResource
	networkTopologyDatabaseResource = apiclient.NetworkTopologyDatabaseResource
	networkProxyResource            = apiclient.NetworkProxyResource
	networkProxyDomainResource      = apiclient.NetworkProxyDomainResource
	projectTopologyResource         = apiclient.ProjectTopologyResource
	projectTopologyNodeResource     = apiclient.ProjectTopologyNodeResource
	projectTopologyNodeStatus       = apiclient.ProjectTopologyNodeStatus
	projectTopologyEdgeResource     = apiclient.ProjectTopologyEdgeResource
	claimOrphanedContainerRequest   = apiclient.ClaimOrphanedContainerRequest
	firewallRuleResource            = apiclient.FirewallRuleResource
	hostFirewallResource            = apiclient.HostFirewallResource
	apiclientHostFirewallPort       = apiclient.HostFirewallPort
	createFirewallRuleRequest       = apiclient.CreateFirewallRuleRequest
	appStreamResource               = apiclient.AppStreamResource
	createAppStreamRequest          = apiclient.CreateAppStreamRequest

	notificationChannelResource      = apiclient.NotificationChannelResource
	createNotificationChannelRequest = apiclient.CreateNotificationChannelRequest
	updateNotificationChannelRequest = apiclient.UpdateNotificationChannelRequest
	testNotificationChannelRequest   = apiclient.TestNotificationChannelRequest
	notificationDeliveryResource     = apiclient.NotificationDeliveryResource
	pushSubscriptionResource         = apiclient.PushSubscriptionResource
	logDrainResource                 = apiclient.LogDrainResource
	setLogDrainRequest               = apiclient.SetLogDrainRequest

	scheduledTaskResource = apiclient.ScheduledTaskResource
	scheduledTaskRequest  = apiclient.ScheduledTaskRequest
	jobRunResource        = apiclient.JobRunResource

	alertRuleResource      = apiclient.AlertRuleResource
	createAlertRuleRequest = apiclient.CreateAlertRuleRequest
	updateAlertRuleRequest = apiclient.UpdateAlertRuleRequest

	deployNotifyTargetResource      = apiclient.DeployNotifyTargetResource
	createDeployNotifyTargetRequest = apiclient.CreateDeployNotifyTargetRequest

	featureFlagResource  = apiclient.FeatureFlagResource
	featureFlagRequest   = apiclient.FeatureFlagRequest
	evaluateFlagResource = apiclient.EvaluateFlagResource

	systemStatusResource        = apiclient.SystemStatusResource
	doctorCheckResource         = apiclient.DoctorCheckResource
	systemDoctorResource        = apiclient.SystemDoctorResource
	openAPISpecResource         = apiclient.OpenAPISpecResource
	openAPIRouteResource        = apiclient.OpenAPIRouteResource
	containerResource           = apiclient.ContainerResource
	containerPortResource       = apiclient.ContainerPortResource
	updatesResource             = apiclient.UpdatesResource
	updatePreflight             = apiclient.UpdatePreflight
	releaseHistory              = apiclient.ReleaseHistory
	upgradeHistory              = apiclient.UpgradeHistory
	serverReadiness             = apiclient.ServerReadiness
	upgradeHistoryEntry         = apiclient.UpgradeHistoryItem
	releaseHistoryItem          = apiclient.ReleaseHistoryItem
	rollbackPlan                = apiclient.RollbackPlan
	changelogResource           = apiclient.ChangelogResource
	changelogEntryResource      = apiclient.ChangelogEntryResource
	nodeResource                = apiclient.NodeResource
	setNodeWorkloadsRequest     = apiclient.SetNodeWorkloadsRequest
	createNodeJoinTokenResponse = apiclient.CreateNodeJoinTokenResponse
	drainNodeResponse           = apiclient.DrainNodeResponse
	nodePatchStatusResource     = apiclient.NodePatchStatusResource
	nodeStatusEventResource     = apiclient.NodeStatusEventResource
	nodeAlertStatusResource     = apiclient.NodeAlertStatusResource
	meshStatusResource          = apiclient.MeshStatusResource
	meshPeerResource            = apiclient.MeshPeerResource
	meshRotationResource        = apiclient.MeshRotationResource
	rotateKeyResponse           = apiclient.RotateKeyResponse
	rejoinMeshResponse          = apiclient.RejoinMeshResponse

	nodeProviderResource             = apiclient.NodeProviderResource
	nodeProviderTestResult           = apiclient.NodeProviderTestResult
	nodeProviderRegionResource       = apiclient.NodeProviderRegionResource
	nodeProviderSizeResource         = apiclient.NodeProviderSizeResource
	nodeProvisionResource            = apiclient.NodeProvisionResource
	setNodeProviderCredentialRequest = apiclient.SetNodeProviderCredentialRequest
	createNodeProvisionRequest       = apiclient.CreateNodeProvisionRequest

	sshNodeProvisionResource          = apiclient.SSHNodeProvisionResource
	createSSHNodeProvisionAuthRequest = apiclient.SSHNodeProvisionAuthRequest
	createSSHNodeProvisionRequest     = apiclient.CreateSSHNodeProvisionRequest
	sshProvisionPreflightRequest      = apiclient.SSHProvisionPreflightRequest
	sshProvisionPreflight             = apiclient.SSHProvisionPreflight

	organizationResource             = apiclient.OrganizationResource
	createOrganizationRequest        = apiclient.CreateOrganizationRequest
	projectResource                  = apiclient.ProjectResource
	projectRestartResponse           = apiclient.ProjectRestartResponse
	createProjectRequest             = apiclient.CreateProjectRequest
	projectLifecycleResult           = apiclient.ProjectLifecycleResult
	setProjectOrganizationRequest    = apiclient.SetProjectOrganizationRequest
	setAppProjectRequest             = apiclient.SetAppProjectRequest
	setAppNodeRequest                = apiclient.SetAppNodeRequest
	moveAppWithVolumesRequest        = apiclient.MoveAppWithVolumesRequest
	setDatabaseProjectRequest        = apiclient.SetDatabaseProjectRequest
	environmentResource              = apiclient.EnvironmentResource
	createEnvironmentRequest         = apiclient.CreateEnvironmentRequest
	updateEnvironmentRequest         = apiclient.UpdateEnvironmentRequest
	setAppEnvironmentRequest         = apiclient.SetAppEnvironmentRequest
	environmentClonePreviewResource  = apiclient.EnvironmentClonePreviewResource
	environmentCloneAppPreview       = apiclient.EnvironmentCloneAppPreview
	environmentCloneRequest          = apiclient.EnvironmentCloneRequest
	environmentCloneAppInput         = apiclient.EnvironmentCloneAppInput
	environmentCloneResultResource   = apiclient.EnvironmentCloneResultResource
	environmentEnvEntryResource      = apiclient.EnvironmentEnvEntryResource
	environmentEnvDiffEntry          = apiclient.EnvironmentEnvDiffEntry
	environmentCompareSide           = apiclient.EnvironmentCompareSide
	environmentCompareResource       = apiclient.EnvironmentCompareResource
	previewEnvironmentResource       = apiclient.PreviewEnvironmentResource
	previewEphemeralDatabaseResource = apiclient.PreviewEphemeralDatabaseResource
	previewDatabaseIsolationResource = apiclient.PreviewDatabaseIsolationResource
	setPreviewSettingsRequest        = apiclient.SetPreviewSettingsRequest
	previewSettingsResource          = apiclient.PreviewSettingsResource
	sweepPreviewEnvironmentsResult   = apiclient.SweepPreviewEnvironmentsResult

	userResource               = apiclient.UserResource
	createUserRequest          = apiclient.CreateUserRequest
	updateUserAbilitiesRequest = apiclient.UpdateUserAbilitiesRequest
	roleResource               = apiclient.RoleResource

	inviteResource       = apiclient.InviteResource
	createInviteRequest  = apiclient.CreateInviteRequest
	createInviteResponse = apiclient.CreateInviteResponse

	policyResource           = apiclient.PolicyResource
	policyAttachmentResource = apiclient.PolicyAttachmentResource
	policyRequest            = apiclient.PolicyRequest
	attachPolicyRequest      = apiclient.AttachPolicyRequest

	deviceStartRequest  = apiclient.DeviceStartRequest
	deviceStartResponse = apiclient.DeviceStartResponse
	deviceTokenResponse = apiclient.DeviceTokenResponse

	secretKeyResource   = apiclient.SecretKeyResource
	gitSourceResource   = apiclient.GitSourceResource
	setGitSourceRequest = apiclient.SetGitSourceRequest

	integrationCatalogEntryResource = apiclient.IntegrationCatalogEntryResource
	appIntegrationResource          = apiclient.AppIntegrationResource

	sharedEnvVarResource = apiclient.SharedEnvVarResource

	webhookDeliveryResource     = apiclient.WebhookDeliveryResource
	replayWebhookDeliveryResult = apiclient.ReplayWebhookDeliveryResult

	deployCompareResource = apiclient.DeployCompareResource
	releaseNotesResource  = apiclient.ReleaseNotesResource
	deployCompareSide     = apiclient.DeployCompareSide
	deployCompareField    = apiclient.DeployCompareField

	promotePreviewResource = apiclient.PromotePreviewResource
	promotePreviewSide     = apiclient.PromotePreviewSide
	promoteAppRequest      = apiclient.PromoteAppRequest

	auditLogEntryResource = apiclient.AuditLogEntryResource
	listAuditLogOptions   = apiclient.ListAuditLogOptions
	purgeAuditLogResult   = apiclient.PurgeAuditLogResult

	rotateMasterKeyResult = apiclient.RotateMasterKeyResult
	secretBindingStatus   = apiclient.SecretBindingStatus
	secretRebindResult    = apiclient.SecretRebindResult

	systemPruneResult = apiclient.SystemPruneResult

	orphanReport                 = apiclient.OrphanReport
	orphanFinding                = apiclient.OrphanFinding
	orphanedVolumeResource       = apiclient.OrphanedVolumeResource
	cleanupOrphanedVolumesResult = apiclient.CleanupOrphanedVolumesResult

	oauthProviderSettingsResource      = apiclient.OAuthProviderSettingsResource
	updateOAuthProviderSettingsRequest = apiclient.UpdateOAuthProviderSettingsRequest
	emailSettingsResource              = apiclient.EmailSettingsResource
	ingressSettingsResource            = apiclient.IngressSettingsResource
	httpsStatusResource                = apiclient.HTTPSStatusResource
	imageAutoUpdateResource            = apiclient.ImageAutoUpdateResource
	buildHookResource                  = apiclient.BuildHookResource
	appSleepResource                   = apiclient.AppSleepResource
	canaryResource                     = apiclient.CanaryResource
	enableHTTPSRequest                 = apiclient.EnableHTTPSRequest
	dashboardURLResource               = apiclient.DashboardURLResource
	updateSettingsResource             = apiclient.UpdateSettingsResource
	aiAssistantSettingsResource        = apiclient.AIAssistantSettingsResource
	appStorageResource                 = apiclient.AppStorageResource
	certificateResource                = apiclient.CertificateResource
	renewCertificateResource           = apiclient.RenewCertificateResource
	gitProviderResource                = apiclient.GitProviderResource
	gitHubAppStatusResource            = apiclient.GitHubAppStatusResource
	gitHubAppRepoResource              = apiclient.GitHubAppRepoResource
	gitHubAppRepoListResource          = apiclient.GitHubAppRepoListResource
	gitHubAppRepoListErrResource       = apiclient.GitHubAppRepoListErrResource
	gitHubAppInstallationResource      = apiclient.GitHubAppInstallationResource
	gitHubAppInstallationListResource  = apiclient.GitHubAppInstallationListResource
	gitAppBranchResource               = apiclient.GitAppBranchResource
	useRepoAsSourceRequest             = apiclient.UseRepoAsSourceRequest
	useGitHubRepoAsSourceResponse      = apiclient.UseGitHubRepoAsSourceResponse
	serviceTemplateListItem            = apiclient.ServiceTemplateListItem
	serviceTemplateDetail              = apiclient.ServiceTemplateDetail
	customTemplateListItem             = apiclient.CustomTemplateListItem
	staticSiteResource                 = apiclient.StaticSiteResource

	tagResource         = apiclient.TagResource
	createTagRequest    = apiclient.CreateTagRequest
	attachAppTagRequest = apiclient.AttachAppTagRequest
	tagAppResource      = apiclient.TagAppResource

	nodeResourceUsageResource  = apiclient.NodeResourceUsageResource
	fleetResourceUsageRollup   = apiclient.FleetResourceUsageRollup
	fleetResourceUsageResource = apiclient.FleetResourceUsageResource

	capacityForecastMetric       = apiclient.CapacityForecastMetric
	nodeCapacityForecastResource = apiclient.NodeCapacityForecastResource
)

// NewClient identifies requests as this CLI so the audit log can tell CLI actions apart.
func NewClient(baseURL, token string, extra ...apiclient.Option) *Client {
	return apiclient.NewClient(baseURL, token, append([]apiclient.Option{apiclient.WithUserAgent("levelrail-cli/" + version.Version)}, extra...)...)
}

// Aliases apiclient's helpers; this and CoolifyClient share one URL and error-body helper.
func pathEscape(s string) string {
	return apiclient.PathEscape(s)
}

func extractErrorMessage(data []byte) string {
	return apiclient.ExtractErrorMessage(data)
}
