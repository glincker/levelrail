---
description: Complete HTTP API reference for the control plane, organized by resource group with 296 endpoints.
---

# REST API Reference

Exhaustive route inventory (296 routes total) for Levelrail's control plane HTTP API, organized by resource group matching `docs/feature-catalog.md`.

## System

System endpoints for:
- Status checks and diagnostics
- Admin operations
- Configuration retrieval and management

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /healthz | Public | handleHealthz |
| GET | /api/v1/brand | Public | handleBrand |
| GET | /api/v1/dev-mode | Public | handleDevMode |
| GET | /api/v1/system/status | AbilityRead | handleSystemStatus |
| GET | /api/v1/system/doctor | AbilityRead | handleSystemDoctor |
| GET | /api/v1/system/containers | AbilityRead | handleListContainers |
| POST | /api/v1/system/prune | AbilityRoot | handleSystemPrune |
| POST | /api/v1/system/master-key/rotate | AbilityRoot | handleRotateMasterKey |
| GET | /api/v1/onboarding | AbilityRead | handleGetOnboardingState |
| POST | /api/v1/onboarding/complete | AbilityWrite | handleCompleteOnboarding |
| GET | /api/v1/updates | AbilityRead | handleGetUpdates |

## Auth / 2FA / Users / Roles / IAM / Device Auth / OAuth

::: details 43 endpoints for authentication, two-factor auth, user management, IAM, device login, and OAuth integration

Endpoints for:
- Authentication and session management
- Two-factor authentication setup and verification
- User and role management
- IAM policies and attachment
- Device login flows
- OAuth provider configuration

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| POST | /api/v1/auth/login | Public | handleLogin |
| POST | /api/v1/auth/register | Public | handleRegister |
| POST | /api/v1/auth/logout | Session | handleLogout |
| PUT | /api/v1/auth/password | Session | handleChangePassword |
| GET | /api/v1/auth/session | Session | handleGetSession |
| POST | /api/v1/auth/sessions/revoke-others | Session | handleRevokeOtherSessions |
| GET | /api/v1/auth/2fa | Session | handleGetTwoFactorStatus |
| POST | /api/v1/auth/2fa/setup | Session | handleSetupTwoFactor |
| POST | /api/v1/auth/2fa/confirm | Session | handleConfirmTwoFactor |
| POST | /api/v1/auth/2fa/disable | Session | handleDisableTwoFactor |
| POST | /api/v1/auth/2fa/recovery-codes/regenerate | Session | handleRegenerateRecoveryCodes |
| POST | /api/v1/auth/2fa/verify | Public | handleVerifyTwoFactor |
| POST | /api/v1/auth/users | AbilityRoot | handleCreateUser |
| GET | /api/v1/users | AbilityRead | handleListUsers |
| DELETE | /api/v1/users/{id} | AbilityRoot | handleDeleteUser |
| PUT | /api/v1/users/{id}/abilities | AbilityRoot | handleUpdateUserAbilities |
| GET | /api/v1/roles | AbilityRead | handleListRoles |
| POST | /api/v1/invites | AbilityWrite | handleCreateInvite |
| GET | /api/v1/invites | AbilityRead | handleListInvites |
| DELETE | /api/v1/invites/{id} | AbilityWrite | handleRevokeInvite |
| POST | /api/v1/invites/accept | Public | handleAcceptInvite |
| POST | /api/v1/iam/policies | AbilityRoot | handleCreatePolicy |
| GET | /api/v1/iam/policies | AbilityRead | handleListPolicies |
| GET | /api/v1/iam/policies/{id} | AbilityRead | handleGetPolicy |
| PUT | /api/v1/iam/policies/{id} | AbilityRoot | handleUpdatePolicy |
| DELETE | /api/v1/iam/policies/{id} | AbilityRoot | handleDeletePolicy |
| GET | /api/v1/iam/policies/{id}/attachments | AbilityRead | handleListPolicyAttachments |
| POST | /api/v1/iam/policies/{id}/attachments | AbilityRoot | handleAttachPolicy |
| DELETE | /api/v1/iam/policies/{id}/attachments/{principal_type}/{principal_id} | AbilityRoot | handleDetachPolicy |
| POST | /api/v1/auth/device/start | Public | handleDeviceAuthStart |
| POST | /api/v1/auth/device/token | Public | handleDeviceAuthToken |
| GET | /api/v1/auth/device/requests | Session | handleListDeviceAuthRequests |
| POST | /api/v1/auth/device/{user_code}/approve | Session | handleApproveDeviceAuthRequest |
| POST | /api/v1/auth/device/{user_code}/deny | Session | handleDenyDeviceAuthRequest |
| GET | /api/v1/auth/oauth/providers | Public | handleListPublicOAuthProviders |
| GET | /api/v1/auth/oauth/{provider}/start | Public | handleOAuthStart |
| GET | /api/v1/auth/oauth/{provider}/callback | Public | handleOAuthCallback |
| GET | /api/v1/auth/oauth/{provider}/link/start | Session | handleOAuthLinkStart |
| GET | /api/v1/settings/oauth | AbilityRead | handleListOAuthSettings |
| PUT | /api/v1/settings/oauth/{provider} | AbilityRoot | handleUpdateOAuthProviderSettings |
| POST | /api/v1/auth/forgot-password | Public | handleForgotPassword |
| POST | /api/v1/auth/reset-password | Public | handleResetPassword |
| POST | /api/v1/auth/tokens | Session | handleCreateToken |
| GET | /api/v1/auth/tokens | Session | handleListTokens |
| DELETE | /api/v1/auth/tokens/{id} | Session | handleRevokeToken |

:::

## Apps CRUD / Lifecycle / Deploy

::: details 32 endpoints for app management, deployment, lifecycle control, and diagnostics

Endpoints for:
- Application creation, retrieval, update, and deletion
- Deployment triggers and history
- Application control (restart, stop, start)
- Logs, metrics, and diagnostics
- Build operations and deployment comparisons
- Image and network information

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/apps | AbilityRead | handleListApps |
| POST | /api/v1/apps | AbilityWrite | handleCreateApp |
| GET | /api/v1/apps/{name} | AbilityRead | handleGetApp |
| PUT | /api/v1/apps/{name} | AbilityWrite | handleUpdateApp |
| DELETE | /api/v1/apps/{name} | AbilityWrite | handleDeleteApp |
| GET | /api/v1/apps/{name}/group | AbilityRead | handleGetAppGroup |
| GET | /api/v1/apps/{name}/hook-runs | AbilityRead | handleGetAppHookRuns |
| POST | /api/v1/apps/{name}/compose | AbilityDeploy | handleDeployCompose |
| GET | /api/v1/service-templates | AbilityRead | handleListServiceTemplates |
| GET | /api/v1/service-templates/{id} | AbilityRead | handleGetServiceTemplate |
| POST | /api/v1/apps/{name}/clone | AbilityWrite | handleCloneApp |
| PUT | /api/v1/apps/{name}/node | AbilityRoot | handleSetAppNode |
| POST | /api/v1/apps/{name}/deploys | AbilityDeploy | handleTriggerDeploy |
| GET | /api/v1/apps/{name}/deploys | AbilityRead | handleDeployHistory |
| POST | /api/v1/apps/{name}/restart | AbilityDeploy | handleRestartApp |
| POST | /api/v1/apps/{name}/stop | AbilityDeploy | handleStopApp |
| POST | /api/v1/apps/{name}/start | AbilityDeploy | handleStartApp |
| POST | /api/v1/apps/{name}/exec | AbilityRoot | handleExecApp |
| GET | /api/v1/apps/{name}/terminal | AbilityRoot | handleAppTerminal |
| GET | /api/v1/apps/{name}/deploy-attempts | AbilityRead | handleListDeployAttempts |
| GET | /api/v1/apps/{name}/deploys/compare | AbilityRead | handleCompareDeploys |
| GET | /api/v1/apps/{name}/promote/preview | AbilityRead | handlePromotePreview |
| POST | /api/v1/apps/{name}/promote | AbilityDeploy | handlePromoteApp |
| GET | /api/v1/apps/{name}/deploys/{deployId}/logs | AbilityRead | handleDeployLogStream |
| GET | /api/v1/apps/{name}/deploys/{deployId}/logs/download | AbilityRead | handleDownloadDeployLog |
| GET | /api/v1/apps/{name}/diagnose | AbilityRead | handleDiagnoseApp |
| GET | /api/v1/apps/{name}/resource-recommendation | AbilityRead | handleAppResourceRecommendation |
| POST | /api/v1/apps/{name}/builds | AbilityDeploy | handleTriggerBuild |
| POST | /api/v1/apps/{name}/deploy-spec | AbilityDeploy | handleDeploySpec |
| POST | /api/v1/git/branches | AbilityDeploy | handleListGitBranches |
| GET | /api/v1/apps/{name}/images | AbilityRead | handleListImages |
| GET | /api/v1/apps/{name}/network | AbilityRead | handleGetAppNetwork |

:::

## Secrets / Git Source / Webhooks / Preview Environments

Endpoints for:
- Encrypted secrets management and locking
- Git repository connections and configuration
- Webhook delivery history and replay
- Preview environment configuration and lifecycle

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| PUT | /api/v1/apps/{name}/secrets/{key} | AbilityWriteSensitive | handleSetSecret |
| GET | /api/v1/apps/{name}/secrets | AbilityRead | handleListSecrets |
| POST | /api/v1/apps/{name}/secrets/{key}/lock | AbilityWriteSensitive | handleSetSecretLock |
| GET | /api/v1/apps/{name}/git-source | AbilityRead | handleGetGitSource |
| PUT | /api/v1/apps/{name}/git-source | AbilityWriteSensitive | handleSetGitSource |
| DELETE | /api/v1/apps/{name}/git-source | AbilityWriteSensitive | handleDeleteGitSource |
| POST | /api/v1/webhooks/github/{name} | Public | handleGitPushWebhook |
| GET | /api/v1/apps/{name}/webhook-deliveries | AbilityRead | handleListWebhookDeliveries |
| POST | /api/v1/apps/{name}/webhook-deliveries/{id}/replay | AbilityDeploy | handleReplayWebhookDelivery |
| PUT | /api/v1/apps/{name}/preview-settings | AbilityWriteSensitive | handleSetPreviewEnabled |
| GET | /api/v1/apps/{name}/previews | AbilityRead | handleListPreviewEnvironments |
| POST | /api/v1/apps/{name}/previews/{number}/teardown | AbilityDeploy | handleTeardownPreviewEnvironment |
| POST | /api/v1/previews/sweep | AbilityDeploy | handleSweepPreviewEnvironments |

## Telemetry

Endpoints for:
- Metrics queries with filtering and aggregation
- Log retrieval, search, and live streaming
- Resource usage ranking and monitoring

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/apps/{name}/metrics | AbilityRead | handleQueryMetrics |
| GET | /api/v1/apps/{name}/logs | AbilityRead | handleQueryLogs |
| GET | /api/v1/apps/resource-usage | AbilityRead | handleAppResourceUsage |
| GET | /api/v1/apps/{name}/logs/stream | AbilityRead | handleLiveLogStream |
| GET | /api/v1/apps/{name}/logs/download | AbilityRead | handleDownloadLogs |

## Alerts / Scheduled Tasks / Feature Flags / Notification Channels

Endpoints for:
- Alert rule creation and management
- Scheduled task configuration and execution
- Feature flag configuration and evaluation
- Notification channel management and delivery tracking

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| POST | /api/v1/apps/{name}/alerts | AbilityWrite | handleCreateAlertRule |
| GET | /api/v1/apps/{name}/alerts | AbilityRead | handleListAlertRules |
| PUT | /api/v1/apps/{name}/alerts/{id} | AbilityWrite | handleUpdateAlertRule |
| DELETE | /api/v1/apps/{name}/alerts/{id} | AbilityWrite | handleDeleteAlertRule |
| POST | /api/v1/apps/{name}/scheduled-tasks | AbilityWrite | handleCreateScheduledTask |
| GET | /api/v1/apps/{name}/scheduled-tasks | AbilityRead | handleListScheduledTasks |
| GET | /api/v1/apps/{name}/scheduled-tasks/{id} | AbilityRead | handleGetScheduledTask |
| PUT | /api/v1/apps/{name}/scheduled-tasks/{id} | AbilityWrite | handleUpdateScheduledTask |
| DELETE | /api/v1/apps/{name}/scheduled-tasks/{id} | AbilityWrite | handleDeleteScheduledTask |
| POST | /api/v1/apps/{name}/scheduled-tasks/{id}/run | AbilityDeploy | handleRunScheduledTaskNow |
| POST | /api/v1/apps/{name}/flags | AbilityWrite | handleCreateFeatureFlag |
| GET | /api/v1/apps/{name}/flags | AbilityRead | handleListFeatureFlags |
| GET | /api/v1/apps/{name}/flags/{id} | AbilityRead | handleGetFeatureFlag |
| PUT | /api/v1/apps/{name}/flags/{id} | AbilityWrite | handleUpdateFeatureFlag |
| DELETE | /api/v1/apps/{name}/flags/{id} | AbilityWrite | handleDeleteFeatureFlag |
| GET | /api/v1/flags/evaluate/{key} | AbilityRead | handleEvaluateFeatureFlag |
| POST | /api/v1/apps/{name}/deploy-notify-targets | AbilityWrite | handleCreateDeployNotifyTarget |
| GET | /api/v1/apps/{name}/deploy-notify-targets | AbilityRead | handleListDeployNotifyTargets |
| DELETE | /api/v1/apps/{name}/deploy-notify-targets/{id} | AbilityWrite | handleDeleteDeployNotifyTarget |
| GET | /api/v1/notification-channels | AbilityRead | handleListNotificationChannels |
| POST | /api/v1/notification-channels | AbilityWrite | handleCreateNotificationChannel |
| PUT | /api/v1/notification-channels/{id} | AbilityWrite | handleUpdateNotificationChannel |
| DELETE | /api/v1/notification-channels/{id} | AbilityWrite | handleDeleteNotificationChannel |
| POST | /api/v1/notification-channels/test | AbilityWrite | handleTestNotificationChannel |
| POST | /api/v1/notification-channels/{id}/test | AbilityWrite | handleTestExistingNotificationChannel |
| GET | /api/v1/notification-channels/{id}/deliveries | AbilityRead | handleListNotificationDeliveries |
| POST | /api/v1/prometheus/read | AbilityRead | handlePrometheusRead |

## Databases CRUD / Engines / Resources

Endpoints for:
- Database creation, retrieval, and deletion
- Engine registry and telemetry
- Resource recommendations and node placement

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/database-engines | AbilityRead | handleListDatabaseEngines |
| GET | /api/v1/databases | AbilityRead | handleListDatabases |
| POST | /api/v1/databases | AbilityWrite | handleCreateDatabase |
| GET | /api/v1/databases/{name} | AbilityRead | handleGetDatabase |
| DELETE | /api/v1/databases/{name} | AbilityWrite | handleDeleteDatabase |
| GET | /api/v1/databases/{name}/status | AbilityRead | handleDatabaseStatus |
| GET | /api/v1/databases/{name}/metrics | AbilityRead | handleQueryDatabaseMetrics |
| GET | /api/v1/databases/{name}/logs | AbilityRead | handleQueryDatabaseLogs |
| GET | /api/v1/databases/{name}/logs/stream | AbilityRead | handleLiveDatabaseLogStream |
| GET | /api/v1/databases/{name}/resource-recommendation | AbilityRead | handleDatabaseResourceRecommendation |
| PUT | /api/v1/databases/{name}/node | AbilityRoot | handleSetDatabaseNode |

## Projects / Organizations / Environments

Endpoints for:
- Project and organization management
- Environment definitions and configuration
- Environment variable and configuration inheritance
- Bulk lifecycle operations (restart, stop, start)

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/projects | AbilityRead | handleListProjects |
| POST | /api/v1/projects | AbilityWrite | handleCreateProject |
| GET | /api/v1/projects/{id} | AbilityRead | handleGetProject |
| DELETE | /api/v1/projects/{id} | AbilityWrite | handleDeleteProject |
| POST | /api/v1/projects/{id}/restart | AbilityDeploy | handleRestartProject |
| POST | /api/v1/projects/{id}/stop | AbilityDeploy | handleStopProject |
| POST | /api/v1/projects/{id}/start | AbilityDeploy | handleStartProject |
| GET | /api/v1/organizations | AbilityRead | handleListOrganizations |
| POST | /api/v1/organizations | AbilityWrite | handleCreateOrganization |
| GET | /api/v1/organizations/{id} | AbilityRead | handleGetOrganization |
| DELETE | /api/v1/organizations/{id} | AbilityWrite | handleDeleteOrganization |
| PUT | /api/v1/projects/{id}/organization | AbilityWrite | handleSetProjectOrganization |
| GET | /api/v1/organizations/{id}/env | AbilityRead | handleGetOrganizationEnv |
| PUT | /api/v1/organizations/{id}/env | AbilityWrite | handleSetOrganizationEnv |
| GET | /api/v1/projects/{id}/environments | AbilityRead | handleListEnvironments |
| POST | /api/v1/projects/{id}/environments | AbilityWrite | handleCreateEnvironment |
| PATCH | /api/v1/environments/{id} | AbilityWrite | handleUpdateEnvironment |
| DELETE | /api/v1/environments/{id} | AbilityWrite | handleDeleteEnvironment |
| PUT | /api/v1/apps/{name}/environment | AbilityWrite | handleSetAppEnvironment |
| GET | /api/v1/environments/{id}/env | AbilityRead | handleGetEnvironmentEnv |
| PUT | /api/v1/environments/{id}/env | AbilityWrite | handleSetEnvironmentEnv |
| GET | /api/v1/projects/{id}/env | AbilityRead | handleGetProjectEnv |
| PUT | /api/v1/projects/{id}/env | AbilityWrite | handleSetProjectEnv |
| PUT | /api/v1/apps/{name}/project | AbilityWrite | handleSetAppProject |
| PUT | /api/v1/databases/{name}/project | AbilityWrite | handleSetDatabaseProject |
| PUT | /api/v1/databases/{name}/resources | AbilityWrite | handleSetDatabaseResources |

## Nodes

Endpoints for:
- Physical infrastructure management and monitoring
- Node health and status
- Workload assignment and placement
- Cordon, drain, and lifecycle operations
- Node-level metrics and patch status

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/nodes | AbilityRoot | handleListNodes |
| GET | /api/v1/nodes/{id} | AbilityRoot | handleGetNode |
| DELETE | /api/v1/nodes/{id} | AbilityRoot | handleDeleteNode |
| PUT | /api/v1/nodes/{id}/workloads | AbilityRoot | handleSetNodeWorkloads |
| POST | /api/v1/nodes/join-tokens | AbilityRoot | handleCreateNodeJoinToken |
| GET | /api/v1/nodes/{id}/health | AbilityRoot | handleGetNodeHealth |
| POST | /api/v1/nodes/{id}/cordon | AbilityRoot | handleCordonNode |
| POST | /api/v1/nodes/{id}/uncordon | AbilityRoot | handleUncordonNode |
| POST | /api/v1/nodes/{id}/drain | AbilityRoot | handleDrainNode |
| GET | /api/v1/nodes/{id}/metrics | AbilityRoot | handleQueryNodeMetrics |
| GET | /api/v1/nodes/{id}/patch-status | AbilityRoot | handleGetNodePatchStatus |

## Ingress / Certificates / Domains / Email / Cloudflare

::: details 41 endpoints for TLS, domains, ingress control, and DNS/Vault integrations

Endpoints for:
- TLS certificate lifecycle and management
- ACME configuration and certificate validation
- Domain routing and DNS configuration
- Domain-level controls (basic auth, maintenance mode, redirects, custom error pages, WAF)
- Email configuration for notifications
- Cloudflare, Route 53, and Vault integrations
- External secret management

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/certificates | AbilityRead | handleListCertificates |
| GET | /api/v1/settings/ingress | AbilityRead | handleGetIngressSettings |
| PUT | /api/v1/settings/ingress | AbilityRoot | handleUpdateIngressSettings |
| GET | /api/v1/settings/ingress/check | AbilityRoot | handleCheckIngressDomain |
| GET | /api/v1/apps/{name}/domains/{domain}/check | AbilityRead | handleCheckDomain |
| GET | /api/v1/apps/{name}/domains/{domain}/auth | AbilityRead | handleGetDomainBasicAuth |
| PUT | /api/v1/apps/{name}/domains/{domain}/auth | AbilityRoot | handleSetDomainBasicAuth |
| DELETE | /api/v1/apps/{name}/domains/{domain}/auth | AbilityRoot | handleClearDomainBasicAuth |
| GET | /api/v1/apps/{name}/domains/{domain}/maintenance | AbilityRead | handleGetDomainMaintenance |
| PUT | /api/v1/apps/{name}/domains/{domain}/maintenance | AbilityDeploy | handleSetDomainMaintenance |
| DELETE | /api/v1/apps/{name}/domains/{domain}/maintenance | AbilityDeploy | handleClearDomainMaintenance |
| GET | /api/v1/apps/{name}/domains/{domain}/redirect | AbilityRead | handleGetDomainRedirect |
| PUT | /api/v1/apps/{name}/domains/{domain}/redirect | AbilityDeploy | handleSetDomainRedirect |
| DELETE | /api/v1/apps/{name}/domains/{domain}/redirect | AbilityDeploy | handleClearDomainRedirect |
| GET | /api/v1/apps/{name}/domains/{domain}/tls-cert | AbilityRead | handleGetDomainTLSCert |
| PUT | /api/v1/apps/{name}/domains/{domain}/tls-cert | AbilityRoot | handleSetDomainTLSCert |
| DELETE | /api/v1/apps/{name}/domains/{domain}/tls-cert | AbilityRoot | handleClearDomainTLSCert |
| GET | /api/v1/apps/{name}/domains/{domain}/waf | AbilityRead | handleGetDomainWAF |
| PUT | /api/v1/apps/{name}/domains/{domain}/waf | AbilityDeploy | handleSetDomainWAF |
| DELETE | /api/v1/apps/{name}/domains/{domain}/waf | AbilityDeploy | handleClearDomainWAF |
| GET | /api/v1/apps/{name}/domains/{domain}/error-pages | AbilityRead | handleGetDomainErrorPages |
| PUT | /api/v1/apps/{name}/domains/{domain}/error-pages | AbilityDeploy | handleSetDomainErrorPage |
| DELETE | /api/v1/apps/{name}/domains/{domain}/error-pages | AbilityDeploy | handleClearDomainErrorPages |
| GET | /api/v1/settings/email | AbilityRead | handleGetEmailSettings |
| PUT | /api/v1/settings/email | AbilityRoot | handleUpdateEmailSettings |
| GET | /api/v1/settings/cloudflare-tunnel | AbilityRead | handleGetCloudflareTunnelSettings |
| PUT | /api/v1/settings/cloudflare-tunnel | AbilityRoot | handleUpdateCloudflareTunnelSettings |
| DELETE | /api/v1/settings/cloudflare-tunnel | AbilityRoot | handleDisconnectCloudflareTunnel |
| GET | /api/v1/settings/cloudflare-dns | AbilityRead | handleGetCloudflareDNSSettings |
| PUT | /api/v1/settings/cloudflare-dns | AbilityRoot | handleUpdateCloudflareDNSSettings |
| DELETE | /api/v1/settings/cloudflare-dns | AbilityRoot | handleDisconnectCloudflareDNS |
| GET | /api/v1/settings/route53-dns | AbilityRead | handleGetRoute53DNSSettings |
| PUT | /api/v1/settings/route53-dns | AbilityRoot | handleUpdateRoute53DNSSettings |
| DELETE | /api/v1/settings/route53-dns | AbilityRoot | handleDisconnectRoute53DNS |
| GET | /api/v1/settings/vault | AbilityRead | handleGetVaultSettings |
| PUT | /api/v1/settings/vault | AbilityRoot | handleUpdateVaultSettings |
| DELETE | /api/v1/settings/vault | AbilityRoot | handleDisconnectVault |
| PUT | /api/v1/apps/{name}/vault-env/{key} | AbilityWrite | handleSetAppVaultEnv |
| DELETE | /api/v1/apps/{name}/vault-env/{key} | AbilityWrite | handleClearAppVaultEnv |
| PUT | /api/v1/apps/{name}/preview-env/{key} | AbilityWrite | handleSetAppPreviewEnvOverride |
| DELETE | /api/v1/apps/{name}/preview-env/{key} | AbilityWrite | handleClearAppPreviewEnvOverride |

:::

## Static Sites / Backup Targets / Registry Credentials

Endpoints for:
- Static site configuration and visibility
- S3-compatible backup target management and testing
- External registry credentials for image pull authentication
- Domain enumeration

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/static-sites | AbilityRead | handleListStaticSites |
| GET | /api/v1/backup-targets | AbilityRead | handleListBackupTargets |
| POST | /api/v1/backup-targets | AbilityWriteSensitive | handleCreateBackupTarget |
| GET | /api/v1/backup-targets/{id} | AbilityRead | handleGetBackupTarget |
| PUT | /api/v1/backup-targets/{id} | AbilityWriteSensitive | handleUpdateBackupTarget |
| DELETE | /api/v1/backup-targets/{id} | AbilityWriteSensitive | handleDeleteBackupTarget |
| POST | /api/v1/backup-targets/{id}/test | AbilityWriteSensitive | handleTestBackupTarget |
| GET | /api/v1/registry-credentials | AbilityRead | handleListRegistryCredentials |
| POST | /api/v1/registry-credentials | AbilityWriteSensitive | handleCreateRegistryCredential |
| GET | /api/v1/registry-credentials/{id} | AbilityRead | handleGetRegistryCredential |
| PUT | /api/v1/registry-credentials/{id} | AbilityWriteSensitive | handleUpdateRegistryCredential |
| DELETE | /api/v1/registry-credentials/{id} | AbilityWriteSensitive | handleDeleteRegistryCredential |
| POST | /api/v1/registry-credentials/{id}/test | AbilityWriteSensitive | handleTestRegistryCredential |
| GET | /api/v1/registry-credentials/{id}/repositories | AbilityReadSensitive | handleListRegistryCredentialRepositories |
| GET | /api/v1/registry-credentials/{id}/tags | AbilityReadSensitive | handleListRegistryCredentialTags |
| GET | /api/v1/domains | AbilityRead | handleListDomains |

## Built-in Container Registry

Endpoints for:
- Registry configuration and management
- Image repository and tag browsing

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/settings/registry | AbilityRead | handleGetRegistrySettings |
| PUT | /api/v1/settings/registry | AbilityRoot | handleUpdateRegistrySettings |
| DELETE | /api/v1/settings/registry | AbilityRoot | handleDisableRegistry |
| GET | /api/v1/registry/repositories | AbilityRead | handleListRegistryRepositories |
| GET | /api/v1/registry/tags | AbilityRead | handleListRegistryTags |

## Git Provider Apps

Endpoints for:
- GitHub App integration and repository browsing
- GitLab App integration and project browsing
- Bitbucket App integration and repository browsing
- OAuth flow management and provider status

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/git-providers | AbilityReadSensitive | handleListGitProviders |
| GET | /api/v1/github-app | AbilityRoot | handleGetGitHubAppStatus |
| DELETE | /api/v1/github-app | AbilityRoot | handleDisconnectGitHubApp |
| PUT | /api/v1/github-app/manual | AbilityRoot | handleConnectGitHubAppManually |
| GET | /api/v1/github-app/register/preview | AbilityRoot | handleGetGitHubAppManifestPreview |
| GET | /api/v1/github-app/register/start | AbilityRoot | handleStartGitHubAppRegistration |
| GET | /api/v1/github-app/callback | AbilityRoot | handleGitHubAppCallback |
| GET | /api/v1/github-app/installed | AbilityRoot | handleGitHubAppInstalled |
| GET | /api/v1/github-app/repos | AbilityReadSensitive | handleListGitHubAppRepos |
| GET | /api/v1/github-app/repos/{owner}/{repo}/branches | AbilityReadSensitive | handleListGitHubAppBranches |
| POST | /api/v1/github-app/repos/{owner}/{repo}/use-as-source | AbilityWriteSensitive | handleUseGitHubRepoAsSource |
| GET | /api/v1/gitlab-app | AbilityRoot | handleGetGitLabAppStatus |
| PUT | /api/v1/gitlab-app | AbilityRoot | handleConnectGitLabApp |
| DELETE | /api/v1/gitlab-app | AbilityRoot | handleDisconnectGitLabApp |
| GET | /api/v1/gitlab-app/connect | AbilityRoot | handleStartGitLabAppConnect |
| GET | /api/v1/gitlab-app/callback | AbilityRoot | handleGitLabAppCallback |
| GET | /api/v1/gitlab-app/projects | AbilityReadSensitive | handleListGitLabAppProjects |
| GET | /api/v1/gitlab-app/projects/{id}/branches | AbilityReadSensitive | handleListGitLabAppBranches |
| POST | /api/v1/gitlab-app/projects/{id}/use-as-source | AbilityWriteSensitive | handleUseGitLabProjectAsSource |
| GET | /api/v1/bitbucket-app | AbilityRoot | handleGetBitbucketAppStatus |
| PUT | /api/v1/bitbucket-app | AbilityRoot | handleConnectBitbucketApp |
| DELETE | /api/v1/bitbucket-app | AbilityRoot | handleDisconnectBitbucketApp |
| GET | /api/v1/bitbucket-app/connect | AbilityRoot | handleStartBitbucketAppConnect |
| GET | /api/v1/bitbucket-app/callback | AbilityRoot | handleBitbucketAppCallback |
| GET | /api/v1/bitbucket-app/repos | AbilityReadSensitive | handleListBitbucketAppRepos |
| GET | /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/branches | AbilityReadSensitive | handleListBitbucketAppBranches |
| POST | /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/use-as-source | AbilityWriteSensitive | handleUseBitbucketRepoAsSource |

## Database Backups / Restore / Clone Restore

Endpoints for:
- Backup history and manual triggers
- Backup verification and scheduled retention
- Database control (stop, start, public access)
- Restore and clone-restore operations

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/backups | AbilityRead | handleListAllBackups |
| POST | /api/v1/databases/{name}/backups | AbilityWriteSensitive | handleTriggerBackup |
| GET | /api/v1/databases/{name}/backups | AbilityRead | handleListBackupHistory |
| GET | /api/v1/databases/{name}/backups/{historyId}/download | AbilityReadSensitive | handleDownloadBackup |
| POST | /api/v1/databases/{name}/backups/{historyId}/verify | AbilityWriteSensitive | handleVerifyBackup |
| GET | /api/v1/databases/{name}/backups/{historyId}/verifications | AbilityRead | handleListBackupVerifications |
| PUT | /api/v1/databases/{name}/backup-schedule | AbilityWriteSensitive | handleSetBackupSchedule |
| DELETE | /api/v1/databases/{name}/backup-schedule | AbilityWriteSensitive | handleClearBackupSchedule |
| PUT | /api/v1/databases/{name}/public-access | AbilityWriteSensitive | handleSetDatabasePublicAccess |
| DELETE | /api/v1/databases/{name}/public-access | AbilityWriteSensitive | handleClearDatabasePublicAccess |
| POST | /api/v1/databases/{name}/stop | AbilityWriteSensitive | handleStopDatabase |
| POST | /api/v1/databases/{name}/start | AbilityWriteSensitive | handleStartDatabase |
| POST | /api/v1/databases/{name}/restore | AbilityRoot | handleTriggerRestore |
| GET | /api/v1/databases/{name}/restores | AbilityRead | handleListRestoreHistory |
| POST | /api/v1/databases/{name}/restore-as-new | AbilityWriteSensitive | handleCloneRestore |
| GET | /api/v1/databases/{name}/clone-restores | AbilityRead | handleListCloneRestores |

## App Volume Backups / Restore

Endpoints for:
- Named volume backup history and scheduling
- Backup verification and retention management
- Volume restore operations (in-place and non-destructive clone)

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| POST | /api/v1/apps/{name}/volumes/{volume}/backups | AbilityWriteSensitive | handleTriggerVolumeBackup |
| GET | /api/v1/apps/{name}/volumes/{volume}/backups | AbilityRead | handleListVolumeBackupHistory |
| GET | /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download | AbilityReadSensitive | handleDownloadVolumeBackup |
| POST | /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verify | AbilityWriteSensitive | handleVerifyVolumeBackup |
| GET | /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verifications | AbilityRead | handleListVolumeBackupVerifications |
| GET | /api/v1/apps/{name}/volumes/{volume}/backup-schedule | AbilityRead | handleGetVolumeBackupSchedule |
| PUT | /api/v1/apps/{name}/volumes/{volume}/backup-schedule | AbilityWriteSensitive | handleSetVolumeBackupSchedule |
| DELETE | /api/v1/apps/{name}/volumes/{volume}/backup-schedule | AbilityWriteSensitive | handleClearVolumeBackupSchedule |
| POST | /api/v1/apps/{name}/volumes/{volume}/restore | AbilityRoot | handleTriggerVolumeRestore |
| GET | /api/v1/apps/{name}/volumes/{volume}/restores | AbilityRead | handleListVolumeRestoreHistory |
| POST | /api/v1/apps/{name}/volumes/{volume}/restore-as-new | AbilityWriteSensitive | handleVolumeCloneRestore |
| GET | /api/v1/apps/{name}/volumes/{volume}/clone-restores | AbilityRead | handleListVolumeCloneRestores |

## App Storage / Database Attach

Endpoints for:
- Object storage bucket attachment and configuration
- Database connectivity for applications

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| PUT | /api/v1/apps/{name}/storage | AbilityWriteSensitive | handleSetAppStorage |
| DELETE | /api/v1/apps/{name}/storage | AbilityWriteSensitive | handleClearAppStorage |
| PUT | /api/v1/apps/{name}/database | AbilityWrite | handleSetAppDatabase |
| DELETE | /api/v1/apps/{name}/database | AbilityWrite | handleClearAppDatabase |
| GET | /api/v1/storage-env-keys | AbilityRead | handleListStorageEnvKeys |

## Audit Log / Log Drain

Endpoints for:
- Fleet-wide audit trail and log management
- External log sink configuration per application

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/audit-log | AbilityRoot | handleListAuditLog |
| POST | /api/v1/audit-log/purge | AbilityRoot | handlePurgeAuditLog |
| GET | /api/v1/apps/{name}/log-drain | AbilityRead | handleGetAppLogDrain |
| PUT | /api/v1/apps/{name}/log-drain | AbilityWriteSensitive | handleSetAppLogDrain |
| DELETE | /api/v1/apps/{name}/log-drain | AbilityWriteSensitive | handleClearAppLogDrain |

## See also

- [Feature Catalog](./feature-catalog.md) - high-level overview of platform capabilities
- [Observability](./observability.md) - metrics, logs, alerts, and telemetry APIs
- [Architecture](./architecture.md) - control plane design and reconciliation patterns
- [CLI Reference](cli-reference.md) - command-line tool for all API operations
