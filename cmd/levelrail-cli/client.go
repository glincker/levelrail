package main

import (
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/version"
)

// Client and every wire-shape type this file used to define now live in
// internal/apiclient, the one HTTP client implementation shared with
// cmd/levelrail-mcp (see that package's own doc comment). These are
// plain type aliases, not a second implementation: every method call
// below dispatches straight to *apiclient.Client.
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

	backupTargetResource            = apiclient.BackupTargetResource
	createBackupTargetRequest       = apiclient.CreateBackupTargetRequest
	updateBackupTargetRequest       = apiclient.UpdateBackupTargetRequest
	registryCredentialResource      = apiclient.RegistryCredentialResource
	createRegistryCredentialRequest = apiclient.CreateRegistryCredentialRequest
	updateRegistryCredentialRequest = apiclient.UpdateRegistryCredentialRequest

	notificationChannelResource      = apiclient.NotificationChannelResource
	createNotificationChannelRequest = apiclient.CreateNotificationChannelRequest
	updateNotificationChannelRequest = apiclient.UpdateNotificationChannelRequest
	testNotificationChannelRequest   = apiclient.TestNotificationChannelRequest
	notificationDeliveryResource     = apiclient.NotificationDeliveryResource
	logDrainResource                 = apiclient.LogDrainResource
	setLogDrainRequest               = apiclient.SetLogDrainRequest

	scheduledTaskResource = apiclient.ScheduledTaskResource
	scheduledTaskRequest  = apiclient.ScheduledTaskRequest

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
	containerResource           = apiclient.ContainerResource
	containerPortResource       = apiclient.ContainerPortResource
	updatesResource             = apiclient.UpdatesResource
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
	previewEnvironmentResource       = apiclient.PreviewEnvironmentResource
	previewEphemeralDatabaseResource = apiclient.PreviewEphemeralDatabaseResource
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
	deployCompareSide     = apiclient.DeployCompareSide
	deployCompareField    = apiclient.DeployCompareField

	promotePreviewResource = apiclient.PromotePreviewResource
	promotePreviewSide     = apiclient.PromotePreviewSide
	promoteAppRequest      = apiclient.PromoteAppRequest

	auditLogEntryResource = apiclient.AuditLogEntryResource
	listAuditLogOptions   = apiclient.ListAuditLogOptions
	purgeAuditLogResult   = apiclient.PurgeAuditLogResult

	rotateMasterKeyResult = apiclient.RotateMasterKeyResult

	systemPruneResult = apiclient.SystemPruneResult

	orphanedVolumeResource       = apiclient.OrphanedVolumeResource
	cleanupOrphanedVolumesResult = apiclient.CleanupOrphanedVolumesResult

	oauthProviderSettingsResource      = apiclient.OAuthProviderSettingsResource
	updateOAuthProviderSettingsRequest = apiclient.UpdateOAuthProviderSettingsRequest
	emailSettingsResource              = apiclient.EmailSettingsResource
	ingressSettingsResource            = apiclient.IngressSettingsResource
	dashboardURLResource               = apiclient.DashboardURLResource
	aiAssistantSettingsResource        = apiclient.AIAssistantSettingsResource
	updateAIAssistantSettingsRequest   = apiclient.UpdateAIAssistantSettingsRequest
	appStorageResource                 = apiclient.AppStorageResource
	certificateResource                = apiclient.CertificateResource
	gitProviderResource                = apiclient.GitProviderResource
	gitHubAppStatusResource            = apiclient.GitHubAppStatusResource
	gitLabAppStatusResource            = apiclient.GitLabAppStatusResource
	bitbucketAppStatusResource         = apiclient.BitbucketAppStatusResource
	gitHubAppRepoResource              = apiclient.GitHubAppRepoResource
	gitAppBranchResource               = apiclient.GitAppBranchResource
	useRepoAsSourceRequest             = apiclient.UseRepoAsSourceRequest
	useGitHubRepoAsSourceResponse      = apiclient.UseGitHubRepoAsSourceResponse
	gitLabAppProjectResource           = apiclient.GitLabAppProjectResource
	bitbucketAppRepoResource           = apiclient.BitbucketAppRepoResource
	giteaAppStatusResource             = apiclient.GiteaAppStatusResource
	giteaAppRepoResource               = apiclient.GiteaAppRepoResource
	serviceTemplateListItem            = apiclient.ServiceTemplateListItem
	serviceTemplateDetail              = apiclient.ServiceTemplateDetail
	staticSiteResource                 = apiclient.StaticSiteResource

	tagResource         = apiclient.TagResource
	createTagRequest    = apiclient.CreateTagRequest
	attachAppTagRequest = apiclient.AttachAppTagRequest
	tagAppResource      = apiclient.TagAppResource

	nodeResourceUsageResource  = apiclient.NodeResourceUsageResource
	fleetResourceUsageRollup   = apiclient.FleetResourceUsageRollup
	fleetResourceUsageResource = apiclient.FleetResourceUsageResource
)

// NewClient builds a Client, identifying every request as this CLI's own
// (internal/api's clientKindFromUserAgent) so the control plane's audit
// log can tell a CLI-driven action apart from a script or the MCP
// server, rather than lumping every bearer-token caller into one bucket.
// See apiclient.NewClient.
func NewClient(baseURL, token string) *Client {
	return apiclient.NewClient(baseURL, token, apiclient.WithUserAgent("levelrail-cli/"+version.Version))
}

// pathEscape and extractErrorMessage alias apiclient's exported
// versions: authclient.go's authSessionClient and coolify_client.go's
// CoolifyClient are each their own distinct HTTP client (neither is a
// bearer-token control-plane caller), but both reuse this same small,
// generic URL/error-body helper rather than each defining its own copy.
func pathEscape(s string) string {
	return apiclient.PathEscape(s)
}

func extractErrorMessage(data []byte) string {
	return apiclient.ExtractErrorMessage(data)
}
