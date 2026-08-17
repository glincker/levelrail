package main

import "github.com/GLINCKER/levelrail/internal/apiclient"

// Client and every wire-shape type this file used to define now live in
// internal/apiclient, the one HTTP client implementation shared with
// cmd/levelrail-mcp (see that package's own doc comment). These are
// plain type aliases, not a second implementation: every method call
// below dispatches straight to *apiclient.Client.
type (
	Client                   = apiclient.Client
	appResource              = apiclient.AppResource
	serviceResources         = apiclient.ServiceResources
	serviceProbe             = apiclient.ServiceProbe
	serviceHealth            = apiclient.ServiceHealth
	buildTriggerRequest      = apiclient.BuildTriggerRequest
	buildTriggerRequestBuild = apiclient.BuildTriggerRequestBuild
	buildTriggerResponse     = apiclient.BuildTriggerResponse
	conditionResource        = apiclient.ConditionResource
	logEntryResource         = apiclient.LogEntryResource
	execRequest              = apiclient.ExecRequest
	execResponse             = apiclient.ExecResponse
	domainResource           = apiclient.DomainResource
	backupHistoryResource    = apiclient.BackupHistoryResource
	restoreHistoryResource   = apiclient.RestoreHistoryResource
	sessionInfoResource      = apiclient.SessionInfoResource
	databaseResource         = apiclient.DatabaseResource
	composeDeployResult      = apiclient.ComposeDeployResult
	apiError                 = apiclient.APIError
	deployTriggerRequest     = apiclient.DeployTriggerRequest
	triggerBackupRequest     = apiclient.TriggerBackupRequest
	triggerRestoreRequest    = apiclient.TriggerRestoreRequest

	notificationChannelResource      = apiclient.NotificationChannelResource
	createNotificationChannelRequest = apiclient.CreateNotificationChannelRequest
	testNotificationChannelRequest   = apiclient.TestNotificationChannelRequest
	logDrainResource                 = apiclient.LogDrainResource
	setLogDrainRequest               = apiclient.SetLogDrainRequest

	scheduledTaskResource    = apiclient.ScheduledTaskResource
	scheduledTaskRequest     = apiclient.ScheduledTaskRequest
	scheduledTaskRunResource = apiclient.ScheduledTaskRunResource
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
