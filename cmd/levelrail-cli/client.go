package main

import "github.com/GLINCKER/levelrail/internal/apiclient"

// Client and every wire-shape type this file used to define now live in
// internal/apiclient, the one HTTP client implementation shared with
// cmd/levelrail-mcp (see that package's own doc comment). These are
// plain type aliases, not a second implementation: every method call
// below dispatches straight to *apiclient.Client.
type (
	Client                          = apiclient.Client
	appResource                     = apiclient.AppResource
	serviceResources                = apiclient.ServiceResources
	serviceProbe                    = apiclient.ServiceProbe
	serviceHealth                   = apiclient.ServiceHealth
	buildTriggerRequest             = apiclient.BuildTriggerRequest
	buildTriggerRequestBuild        = apiclient.BuildTriggerRequestBuild
	buildTriggerResponse            = apiclient.BuildTriggerResponse
	conditionResource               = apiclient.ConditionResource
	networkResource                 = apiclient.NetworkResource
	logEntryResource                = apiclient.LogEntryResource
	execRequest                     = apiclient.ExecRequest
	execResponse                    = apiclient.ExecResponse
	domainResource                  = apiclient.DomainResource
	cloudflareDNSResource           = apiclient.CloudflareDNSResource
	updateCloudflareDNSRequest      = apiclient.UpdateCloudflareDNSRequest
	cloudflareTunnelResource        = apiclient.CloudflareTunnelResource
	updateCloudflareTunnelRequest   = apiclient.UpdateCloudflareTunnelRequest
	domainBasicAuthResource         = apiclient.DomainBasicAuthResource
	setDomainBasicAuthRequest       = apiclient.SetDomainBasicAuthRequest
	backupHistoryResource           = apiclient.BackupHistoryResource
	backupVerificationResource      = apiclient.BackupVerificationResource
	restoreHistoryResource          = apiclient.RestoreHistoryResource
	sessionInfoResource             = apiclient.SessionInfoResource
	databaseResource                = apiclient.DatabaseResource
	databaseEngineResource          = apiclient.DatabaseEngineResource
	setDatabaseResourcesRequest     = apiclient.SetDatabaseResourcesRequest
	databasePublicAccessResource    = apiclient.DatabasePublicAccessResource
	setDatabasePublicAccessRequest  = apiclient.SetDatabasePublicAccessRequest
	composeDeployResult             = apiclient.ComposeDeployResult
	apiError                        = apiclient.APIError
	deployTriggerRequest            = apiclient.DeployTriggerRequest
	triggerBackupRequest            = apiclient.TriggerBackupRequest
	backupScheduleResource          = apiclient.BackupScheduleResource
	setBackupScheduleRequest        = apiclient.SetBackupScheduleRequest
	triggerRestoreRequest           = apiclient.TriggerRestoreRequest
	setAppDatabaseRequest           = apiclient.SetAppDatabaseRequest
	appDatabaseResource             = apiclient.AppDatabaseResource
	appStatusSummary                = apiclient.AppStatusSummary
	appGroupResource                = apiclient.AppGroupResource
	deploySpecServiceBuild          = apiclient.DeploySpecServiceBuild
	deploySpecServiceEnv            = apiclient.DeploySpecServiceEnv
	deploySpecService               = apiclient.DeploySpecService
	deploySpecRequest               = apiclient.DeploySpecRequest
	deploySpecServiceResult         = apiclient.DeploySpecServiceResult
	deploySpecResult                = apiclient.DeploySpecResult
	diagnosisResource               = apiclient.DiagnosisResource
	diagnosisSignal                 = apiclient.DiagnosisSignal
	resourceRecommendationResource  = apiclient.ResourceRecommendationResource
	dimensionRecommendationResource = apiclient.DimensionRecommendationResource

	backupTargetResource            = apiclient.BackupTargetResource
	createBackupTargetRequest       = apiclient.CreateBackupTargetRequest
	updateBackupTargetRequest       = apiclient.UpdateBackupTargetRequest
	registryCredentialResource      = apiclient.RegistryCredentialResource
	createRegistryCredentialRequest = apiclient.CreateRegistryCredentialRequest
	updateRegistryCredentialRequest = apiclient.UpdateRegistryCredentialRequest

	notificationChannelResource      = apiclient.NotificationChannelResource
	createNotificationChannelRequest = apiclient.CreateNotificationChannelRequest
	testNotificationChannelRequest   = apiclient.TestNotificationChannelRequest
	notificationDeliveryResource     = apiclient.NotificationDeliveryResource
	logDrainResource                 = apiclient.LogDrainResource
	setLogDrainRequest               = apiclient.SetLogDrainRequest

	scheduledTaskResource = apiclient.ScheduledTaskResource
	scheduledTaskRequest  = apiclient.ScheduledTaskRequest

	featureFlagResource  = apiclient.FeatureFlagResource
	featureFlagRequest   = apiclient.FeatureFlagRequest
	evaluateFlagResource = apiclient.EvaluateFlagResource

	systemStatusResource        = apiclient.SystemStatusResource
	doctorCheckResource         = apiclient.DoctorCheckResource
	systemDoctorResource        = apiclient.SystemDoctorResource
	nodeResource                = apiclient.NodeResource
	setNodeWorkloadsRequest     = apiclient.SetNodeWorkloadsRequest
	createNodeJoinTokenResponse = apiclient.CreateNodeJoinTokenResponse
	drainNodeResponse           = apiclient.DrainNodeResponse
	nodePatchStatusResource     = apiclient.NodePatchStatusResource

	organizationResource           = apiclient.OrganizationResource
	createOrganizationRequest      = apiclient.CreateOrganizationRequest
	projectResource                = apiclient.ProjectResource
	createProjectRequest           = apiclient.CreateProjectRequest
	setProjectOrganizationRequest  = apiclient.SetProjectOrganizationRequest
	setAppProjectRequest           = apiclient.SetAppProjectRequest
	setDatabaseProjectRequest      = apiclient.SetDatabaseProjectRequest
	environmentResource            = apiclient.EnvironmentResource
	createEnvironmentRequest       = apiclient.CreateEnvironmentRequest
	setAppEnvironmentRequest       = apiclient.SetAppEnvironmentRequest
	previewEnvironmentResource     = apiclient.PreviewEnvironmentResource
	setPreviewEnabledRequest       = apiclient.SetPreviewEnabledRequest
	sweepPreviewEnvironmentsResult = apiclient.SweepPreviewEnvironmentsResult

	userResource               = apiclient.UserResource
	createUserRequest          = apiclient.CreateUserRequest
	updateUserAbilitiesRequest = apiclient.UpdateUserAbilitiesRequest
	roleResource               = apiclient.RoleResource

	secretKeyResource   = apiclient.SecretKeyResource
	gitSourceResource   = apiclient.GitSourceResource
	setGitSourceRequest = apiclient.SetGitSourceRequest

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
)

// NewClient builds a Client. See apiclient.NewClient.
func NewClient(baseURL, token string) *Client {
	return apiclient.NewClient(baseURL, token)
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
