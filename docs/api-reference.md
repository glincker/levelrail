---
title: "REST API Reference"
description: Complete HTTP API reference for the control plane, organized by resource group.
"og:image": "https://docs.levelrail.com/public/og/api-reference.png"
"twitter:image": "https://docs.levelrail.com/public/og/api-reference.png"
"og:url": "https://docs.levelrail.com/api-reference"
---

Route inventory for Levelrail's control plane HTTP API, organized by resource group matching `docs/feature-catalog.md`.

The tables below are generated from the route registrations in `internal/api/routes*.go`. After adding or changing a route, run `go run ./scripts/gen-api-reference` (fast and idempotent) and commit the result; `TestAPIReferenceCoversEveryRoute` fails if a registered route is missing. New routes land in the group with the closest path prefix, or in **Other**.

## System

System endpoints for:
- Status checks and diagnostics
- Admin operations
- Configuration retrieval and management

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /healthz | Public | handleHealthz |
| GET | /readyz | Public | handleReadyz |
| GET | /api/v1/brand | Public | handleBrand |
| GET | /api/v1/dev-mode | Public | handleDevMode |
| GET | /api/v1/system/status | AbilityRead | handleSystemStatus |
| GET | /api/v1/system/doctor | AbilityRead | handleSystemDoctor |
| GET | /api/v1/system/containers | AbilityRead | handleListContainers |
| POST | /api/v1/system/prune | AbilityRoot | handleSystemPrune |
| POST | /api/v1/system/backups | AbilityRoot | handleCreateControlPlaneBackup |
| GET | /api/v1/system/backups | AbilityRoot | handleListControlPlaneBackups |
| GET | /api/v1/system/backups/{name}/download | AbilityRoot | handleDownloadControlPlaneBackup |
| POST | /api/v1/system/backups/{name}/verify | AbilityRoot | handleVerifyControlPlaneBackup |
| DELETE | /api/v1/system/backups/{name} | AbilityRoot | handleDeleteControlPlaneBackup |
| GET | /api/v1/system/volumes/orphaned | AbilityRead | handleListOrphanedVolumes |
| POST | /api/v1/system/volumes/orphaned/cleanup | AbilityRoot | handleCleanupOrphanedVolumes |
| POST | /api/v1/system/master-key/rotate | AbilityRoot | handleRotateMasterKey |
| GET | /api/v1/onboarding | AbilityRead | handleGetOnboardingState |
| POST | /api/v1/onboarding/complete | AbilityWrite | handleCompleteOnboarding |
| PUT | /api/v1/onboarding/progress | AbilityRoot | handleUpdateOnboardingProgress |
| GET | /api/v1/updates | AbilityRead | handleGetUpdates |
| GET | /api/v1/system/secrets/binding | AbilityRead | handleGetSecretBinding |
| POST | /api/v1/system/secrets/rebind | AbilityRoot | handleRebindSecrets |
| GET | /api/v1/system/control-plane-dr | AbilityRead | handleGetControlPlaneDR |
| PUT | /api/v1/system/control-plane-dr/settings | AbilityWriteSensitive | handleUpdateControlPlaneDR |
| GET | /api/v1/system/control-plane-dr/backups | AbilityRead | handleListControlPlaneDRBackups |
| POST | /api/v1/system/control-plane-dr/run | AbilityWriteSensitive | handleRunControlPlaneDRBackup |
| POST | /api/v1/system/control-plane-dr/drill | AbilityWriteSensitive | handleRunControlPlaneDRDrill |
| POST | /api/v1/system/control-plane-dr/escrow | AbilityRoot | handleControlPlaneDREscrow |
| POST | /api/v1/system/control-plane-dr/escrow/ack | AbilityWriteSensitive | handleAckControlPlaneDREscrow |
| GET | /api/v1/updates/preflight | AbilityRead | handleUpdatePreflight |
| POST | /api/v1/system/containers/{name}/stop | AbilityRoot | handleStopOrphanedContainer |
| POST | /api/v1/system/containers/{name}/remove | AbilityRoot | handleRemoveOrphanedContainer |
| POST | /api/v1/system/containers/{name}/claim | AbilityWrite | handleClaimOrphanedContainer |
| GET | /api/v1/updates/settings | AbilityRoot | handleGetUpdateSettings |
| PUT | /api/v1/updates/settings | AbilityRoot | handleUpdateSettings |
| GET | /api/v1/system/orphans | AbilityRead | handleListOrphans |
| POST | /api/v1/system/orphans/reap | AbilityRoot | handleReapOrphans |
| GET | /api/v1/updates/releases | AbilityRead | handleReleaseHistory |
| GET | /api/v1/updates/rollback-plan | AbilityRoot | handleRollbackPlan |
| GET | /api/v1/updates/history | AbilityRead | handleUpgradeHistory |
| POST | /api/v1/updates/history/{id}/ack | AbilityWrite | handleAckUpgrade |
| GET | /api/v1/system/reverse-proxy | AbilityRead | handleReverseProxyGuide |
| GET | /api/v1/system/docker-guard | AbilityRead | handleGetDockerGuard |
| PUT | /api/v1/system/docker-guard | AbilityRoot | handleUpdateDockerGuard |
| GET | /api/v1/system/geoip | AbilityRead | handleGeoIPLookup |
| GET | /api/v1/system/proxy-integration | AbilityRead | handleGetProxyIntegration |
| POST | /api/v1/system/proxy-integration/setup | AbilityRoot | handleSetupProxyIntegration |
| POST | /api/v1/system/proxy-integration/apply | AbilityRoot | handleApplyProxyIntegration |
| POST | /api/v1/system/proxy-integration/verify | AbilityRoot | handleVerifyProxyIntegration |
| GET | /api/v1/system/readiness | AbilityRead | handleServerReadiness |
| GET | /api/v1/updates/self-upgrade/plan | AbilityRead | handleSelfUpgradePlan |
| GET | /api/v1/updates/self-upgrade/attempts | AbilityRead | handleSelfUpgradeAttempts |
| POST | /api/v1/updates/self-upgrade | AbilityRoot | handleSelfUpgradeStart |
| GET | /api/v1/updates/schedule | AbilityRead | handleGetUpgradeSchedule |
| POST | /api/v1/updates/schedule | AbilityRoot | handleCreateUpgradeSchedule |
| DELETE | /api/v1/updates/schedule | AbilityRoot | handleCancelUpgradeSchedule |
| GET | /api/v1/updates/auto | AbilityRoot | handleGetUpgradePolicy |
| PUT | /api/v1/updates/auto | AbilityRoot | handleUpdateUpgradePolicy |
| POST | /api/v1/updates/backup-now | AbilityRoot | handleUpgradeBackupNow |
| GET | /api/v1/system/server-address | AbilityRead | handleGetServerAddress |
| PUT | /api/v1/system/server-address | AbilityRoot | handleSetServerAddress |
| GET | /api/v1/system/restart | AbilityRead | handleGetSystemRestart |
| POST | /api/v1/system/restart | AbilityRoot | handleSystemRestart |
| GET | /api/v1/system/cli-console/commands | AbilityRoot | handleCLIConsoleCommands |
| GET | /api/v1/system/cli-console/session | AbilityRoot | handleCLIConsoleSession |
| GET | /api/v1/system/networks | AbilityRead | handleGetSystemNetworks |
| POST | /api/v1/system/networks/prune | AbilityRoot | handlePruneSystemNetworks |
| GET | /api/v1/system/shield/bots | AbilityRead | handleShieldBotList |
| GET | /api/v1/system/cache | AbilityRead | handleSystemCache |
| GET | /api/v1/system/janitor | AbilityRead | handleGetJanitor |
| GET | /api/v1/system/janitor/runs | AbilityRead | handleListJanitorRuns |
| PUT | /api/v1/system/janitor/policy | AbilityRoot | handleUpdateJanitorPolicy |
| POST | /api/v1/system/janitor/run | AbilityRoot | handleRunJanitor |
| GET | /api/v1/system/control-plane-health | AbilityRead | handleGetCPHealth |
| POST | /api/v1/system/control-plane-health/fix | AbilityRoot | handleCPHealthFix |
| GET | /api/v1/system/encryption-keys | AbilityRead | handleGetEncryptionKeys |
| PUT | /api/v1/system/encryption-keys/policy | AbilityRoot | handleUpdateKeyPolicy |
| POST | /api/v1/system/encryption-keys/rotate | AbilityRoot | handleRotateDEK |
| GET | /api/v1/system/control-plane-dr/restore-plan | AbilityRead | handleGetControlPlaneRestorePlan |
| POST | /api/v1/system/control-plane-dr/keys | AbilityWriteSensitive | handleGenerateControlPlaneDRKeypair |
| GET | /api/v1/system/integrity | AbilityRead | handleGetIntegrity |
| POST | /api/v1/system/integrity/check | AbilityRoot | handleIntegrityCheck |
| POST | /api/v1/system/integrity/restore | AbilityRoot | handleIntegrityRestore |
| DELETE | /api/v1/system/integrity/restore | AbilityRoot | handleIntegrityCancelRestore |
| POST | /api/v1/updates/rollback | AbilityRoot | handleRollbackStart |
| GET | /api/v1/system/doctor/checks | AbilityRead | handleListDoctorChecks |
| GET | /api/v1/system/doctor/stream | AbilityRead | handleStreamDoctor |
| POST | /api/v1/system/doctor/checks/{id}/run | AbilityRead | handleRunDoctorCheck |
| GET | /api/v1/updates/restore-points | AbilityRoot | handleRestorePointList |
| POST | /api/v1/updates/restore-points/{id}/restore | AbilityRoot | handleRestorePointRestore |
| GET | /api/v1/system/resilience | AbilityRead | handleGetResilience |
| POST | /api/v1/system/resilience/simulate | AbilityWrite | handleSimulateResilience |
| POST | /api/v1/system/resilience/snapshot | AbilityRoot | handleResilienceFix |
| POST | /api/v1/updates/self-upgrade/attempts/{id}/fix | AbilityWrite | handleSelfUpgradeAttemptFix |
| POST | /api/v1/updates/self-upgrade/attempts/{id}/retry | AbilityWrite | handleSelfUpgradeAttemptRetry |
| POST | /api/v1/updates/self-upgrade/attempts/{id}/rollback | AbilityWrite | handleSelfUpgradeAttemptRollback |
| POST | /api/v1/system/proxy-integration/install | AbilityRoot | handleInstallProxy |
| GET | /api/v1/updates/self-upgrade/status | AbilityRead | handleSelfUpgradeStatus |
| GET | /api/v1/system/footprint | AbilityRead | handleGetIdleFootprint |

## Auth / 2FA / Users / Roles / IAM / Device Auth / OAuth

<Accordion title="123 endpoints for authentication, two-factor auth, user management, IAM, device login, and OAuth integration">

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
| POST | /api/v1/auth/register | Public (setup token) | handleRegister |
| GET | /api/v1/auth/setup-status | Public | handleSetupStatus |
| POST | /api/v1/auth/logout | Public | handleLogout |
| PUT | /api/v1/auth/password | Session | handleChangePassword |
| POST | /api/v1/auth/sessions/revoke-others | Session | handleRevokeOtherSessions |
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
| GET | /api/v1/settings/ai-assistant | AbilityRead | handleGetAIAssistantSettings |
| PUT | /api/v1/settings/ai-assistant | AbilityRoot | handleUpdateAIAssistantSettings |
| GET | /api/v1/settings/deploy-freeze | AbilityRead | handleGetGlobalDeployFreeze |
| PUT | /api/v1/settings/deploy-freeze | AbilityRoot | handlePutGlobalDeployFreeze |
| GET | /api/v1/settings/push-subscriptions/vapid-public-key | Session | handleGetPushVAPIDPublicKey |
| GET | /api/v1/settings/push-subscriptions | Session | handleListPushSubscriptions |
| POST | /api/v1/settings/push-subscriptions | Session | handleCreatePushSubscription |
| DELETE | /api/v1/settings/push-subscriptions/{id} | Session | handleDeletePushSubscription |
| GET | /api/v1/settings/observability | AbilityRead | handleGetObservabilitySettings |
| PUT | /api/v1/settings/observability | AbilityRoot | handleUpdateObservabilitySettings |
| POST | /api/v1/auth/session-links | AbilityRoot | handleMintSessionLink |
| GET | /api/v1/auth/session-links/{token}/consume | Public | handleConsumeSessionLinkGET |
| GET | /api/v1/auth/whoami | Public | handleWhoami |
| POST | /api/v1/roles | AbilityRoot | handleCreateRole |
| PUT | /api/v1/roles/{id} | AbilityRoot | handleUpdateRole |
| DELETE | /api/v1/roles/{id} | AbilityRoot | handleDeleteRole |
| PUT | /api/v1/users/{id}/role | AbilityRoot | handleSetUserRole |
| GET | /api/v1/users/{id}/environment-grants | AbilityRoot | handleGetEnvironmentGrants |
| PUT | /api/v1/users/{id}/environment-grants | AbilityRoot | handlePutEnvironmentGrants |
| GET | /api/v1/iam/policy-templates | AbilityRead | handleListPolicyTemplates |
| POST | /api/v1/iam/policy-templates/{id}/apply | AbilityRoot | handleApplyPolicyTemplate |
| GET | /api/v1/settings/ai-control | AbilityRead | handleGetAIControl |
| PUT | /api/v1/settings/ai-control | AbilityRoot | handleUpdateAIControl |
| POST | /api/v1/settings/ai-control/revoke-agent-tokens | AbilityRoot | handleRevokeAgentTokens |
| GET | /api/v1/auth/device/pending-summary | AbilityRead | handleDevicePendingSummary |
| GET | /api/v1/auth/device/activity | AbilityRead | handleDeviceActivity |
| GET | /api/v1/iam/catalog | AbilityRead | handleIAMCatalog |
| GET | /api/v1/iam/resources | AbilityRead | handleIAMResources |
| POST | /api/v1/iam/resources/match | AbilityRead | handleIAMMatch |
| GET | /api/v1/iam/principals | AbilityRead | handleIAMPrincipals |
| GET | /api/v1/iam/principals/{principal_type}/{principal_id}/effective | AbilityRead | handleIAMEffective |
| GET | /api/v1/iam/simulate | AbilityRead | handleIAMSimulate |
| GET | /api/v1/iam/analyze | AbilityRead | handleIAMAnalyze |
| POST | /api/v1/iam/policies/validate | AbilityRead | handleValidatePolicy |
| POST | /api/v1/iam/preview | AbilityRead | handleIAMPreview |
| GET | /api/v1/iam/policies/{id}/versions | AbilityRead | handleListPolicyVersions |
| POST | /api/v1/iam/policy-templates/{id}/render | AbilityRead | handleRenderPolicyTemplate |
| GET | /api/v1/auth/login-options | Public | handleLoginOptions |
| POST | /api/v1/auth/login-code/request | Public | handleRequestLoginCode |
| POST | /api/v1/auth/login-code/redeem | Public | handleRedeemLoginCode |
| POST | /api/v1/auth/login-approval/poll | Public | handlePollLoginApproval |
| GET | /api/v1/auth/sign-in-requests | Public | handleListSignInRequests |
| POST | /api/v1/auth/sign-in-requests/codes/{id}/reveal | Public | handleRevealLoginCode |
| POST | /api/v1/auth/login-approvals/{id}/approve | Public | handleApproveLoginApproval |
| POST | /api/v1/auth/login-approvals/{id}/deny | Public | handleDenyLoginApproval |
| GET | /api/v1/auth/trusted-devices | Public | handleListTrustedDevices |
| DELETE | /api/v1/auth/trusted-devices/{id} | Public | handleRevokeTrustedDevice |
| GET | /api/v1/settings/auth/code-login | AbilityRead | handleGetCodeLoginSettings |
| PUT | /api/v1/settings/auth/code-login | AbilityRoot | handlePutCodeLoginSettings |
| POST | /api/v1/auth/sign-in-alert/disown | Public | handleDisownSignIn |
| GET | /api/v1/security/posture | AbilityRead | handleSecurityPosture |
| GET | /api/v1/security/policy | AbilityRead | handleGetSecurityPolicy |
| PUT | /api/v1/security/policy | AbilityRoot | handlePutSecurityPolicy |
| GET | /api/v1/security/sessions | Public | handleListSecuritySessions |
| DELETE | /api/v1/security/sessions/{id} | Public | handleRevokeSecuritySession |
| POST | /api/v1/security/sessions/revoke-others | Public | handleRevokeOtherSecuritySessions |
| GET | /api/v1/security/account | Public | handleGetAccountSecurity |
| PUT | /api/v1/security/account | Public | handlePutAccountSecurity |
| GET | /api/v1/settings/database-upgrades | AbilityRead | handleGetPlatformUpgradePolicy |
| PUT | /api/v1/settings/database-upgrades | AbilityRoot | handlePutPlatformUpgradePolicy |
| GET | /api/v1/settings/domain-automation | AbilityRead | handleGetDomainAutomation |
| PUT | /api/v1/settings/domain-automation | AbilityRoot | handleUpdateDomainAutomation |
| PUT | /api/v1/settings/proxy-integration | AbilityRoot | handleUpdateProxyIntegrationSettings |
| GET | /api/v1/auth/step-up | Public | handleStepUpOptions |
| POST | /api/v1/auth/step-up | Public | handleStepUp |
| POST | /api/v1/auth/step-up/passkey/begin | Public | handleStepUpPasskeyBegin |
| POST | /api/v1/auth/step-up/passkey/finish | Public | handleStepUpPasskeyFinish |
| GET | /api/v1/settings/beta-features | AbilityRead | handleListBetaFeatures |
| PUT | /api/v1/settings/beta-features/{feature} | AbilityRoot | handleSetBetaFeature |
| POST | /api/v1/auth/session-links/consume | Public | handleConsumeSessionLink |
| PATCH | /api/v1/auth/profile | Session | handleUpdateProfile |
| GET | /api/v1/auth/my-access | Public | handleMyAccess |
| GET | /api/v1/auth/saml/providers | Public | handleListPublicSAMLProviders |
| GET | /api/v1/auth/saml/{id}/metadata | Public | handleSAMLMetadata |
| GET | /api/v1/auth/saml/{id}/login | Public | handleSAMLLogin |
| POST | /api/v1/auth/saml/{id}/acs | Public | handleSAMLACS |
| GET | /api/v1/security/session-policies | AbilityRoot | handleListSessionPolicies |
| PUT | /api/v1/security/session-policies/{team_id} | AbilityRoot | handlePutSessionPolicy |
| DELETE | /api/v1/security/session-policies/{team_id} | AbilityRoot | handleDeleteSessionPolicy |
| GET | /api/v1/security/session-policy/effective | Public | handleEffectiveSessionPolicy |
| PUT | /api/v1/settings/database-catalog | AbilityRoot | handlePutDatabaseCatalogSettings |
| PUT | /api/v1/users/{id}/password-change-required | AbilityRoot | handleSetPasswordChangeRequired |
| GET | /api/v1/settings/ai-assistant/providers | AbilityRead | handleListAIProviders |
| PUT | /api/v1/settings/ai-assistant/providers/{id} | AbilityRoot | handleSetAIProvider |
| POST | /api/v1/settings/ai-assistant/providers/{id}/test | AbilityRoot | handleTestAIProvider |
| DELETE | /api/v1/settings/ai-assistant/providers/{id} | AbilityRoot | handleDeleteAIProvider |
| GET | /api/v1/settings/ai-assistant/providers/{id}/models | AbilityRoot | handleListAIProviderModels |
| GET | /api/v1/auth/my-activity | Public | handleMyActivity |
| GET | /api/v1/auth/my-credentials | Public | handleMyCredentials |
| GET | /api/v1/auth/preferences | Public | handleGetPreferences |
| PUT | /api/v1/auth/preferences | Public | handlePutPreferences |
| POST | /api/v1/auth/export | Public | handleStartExport |
| GET | /api/v1/auth/export/{id} | Public | handleExportStatus |
| GET | /api/v1/auth/export/{id}/download | Public | handleDownloadExport |
| GET | /api/v1/auth/delete-account/check | Public | handleDeleteAccountCheck |
| DELETE | /api/v1/auth/account | Public | handleDeleteAccount |
| DELETE | /api/v1/auth/link/{provider} | Public | handleUnlinkProvider |
| GET | /api/v1/auth/email-change | Public | handleEmailChangeStatus |
| POST | /api/v1/auth/email-change | Public | handleRequestEmailChange |
| POST | /api/v1/auth/email-change/confirm | Public | handleConfirmEmailChange |
| GET | /api/v1/iam/suggestions | AbilityRead | handleIAMSuggestions |
| GET | /api/v1/security/hsts | AbilityRoot | handleGetHSTS |
| PUT | /api/v1/security/hsts | AbilityRoot | handlePutHSTS |
| GET | /api/v1/security/reachability | AbilityRoot | handleSecurityReachability |
| GET | /api/v1/settings/dns-provider-plugin | AbilityRead | handleGetDNSProviderPlugin |
| PUT | /api/v1/settings/dns-provider-plugin | AbilityRoot | handleSetDNSProviderPlugin |
| GET | /api/v1/settings/server-profile | AbilityRead | handleGetServerProfile |
| PUT | /api/v1/settings/server-profile | AbilityRoot | handleSetServerProfile |
| POST | /api/v1/auth/cli-pairing | Session | handleCreateCLIPairing |
| POST | /api/v1/auth/cli-pairing/redeem | Public | handleRedeemCLIPairing |
| GET | /api/v1/settings/dns-provider | AbilityRead | handleGetDNSProvider |
| PUT | /api/v1/settings/dns-provider | AbilityRoot | handleSetDNSProvider |
| POST | /api/v1/settings/dns-provider/test | AbilityRoot | handleTestDNSProvider |
| GET | /api/v1/settings/google-cloud-dns | AbilityRead | handleGetGoogleCloudDNSSettings |
| PUT | /api/v1/settings/google-cloud-dns | AbilityRoot | handleUpdateGoogleCloudDNSSettings |
| DELETE | /api/v1/settings/google-cloud-dns | AbilityRoot | handleDisconnectGoogleCloudDNS |
| POST | /api/v1/settings/google-cloud-dns/test | AbilityRoot | handleTestGoogleCloudDNS |
| GET | /api/v1/settings/traffic-export/sinks | AbilityRead | handleListTrafficExportSinks |
| POST | /api/v1/settings/traffic-export/sinks | AbilityRoot | handleCreateTrafficExportSink |
| GET | /api/v1/settings/traffic-export/sinks/{id} | AbilityRead | handleGetTrafficExportSink |
| PUT | /api/v1/settings/traffic-export/sinks/{id} | AbilityRoot | handleUpdateTrafficExportSink |
| DELETE | /api/v1/settings/traffic-export/sinks/{id} | AbilityRoot | handleDeleteTrafficExportSink |
| POST | /api/v1/settings/traffic-export/sinks/{id}/test | AbilityRoot | handleTestTrafficExportSink |
| GET | /api/v1/settings/traffic-export/status | AbilityRead | handleTrafficExportStatus |
| GET | /api/v1/settings/runtime | AbilityRead | handleListRuntimeSettings |
| GET | /api/v1/settings/runtime/onboarding | AbilityRead | handleGetRuntimeOnboarding |
| GET | /api/v1/settings/runtime/{key} | AbilityRead | handleGetRuntimeSetting |
| PUT | /api/v1/settings/runtime/{key} | AbilityRoot | handleSetRuntimeSetting |
| DELETE | /api/v1/settings/runtime/{key} | AbilityRoot | handleResetRuntimeSetting |
| POST | /api/v1/settings/runtime/apply | AbilityRoot | handleApplyRuntimeSettings |
| POST | /api/v1/settings/runtime/onboarding/ack | AbilityRoot | handleAckRuntimeOnboarding |
| POST | /api/v1/auth/tokens/cleanup | AbilityRoot | handleCleanupTokens |
| GET | /api/v1/auth/session | Public | handleAuthSession |
| GET | /api/v1/settings/heartbeat | AbilityRoot | handleGetHeartbeat |
| PUT | /api/v1/settings/heartbeat | AbilityRoot | handlePutHeartbeat |
| GET | /api/v1/auth/2fa | Session |  |
| POST | /api/v1/auth/2fa/setup | Session |  |
| POST | /api/v1/auth/2fa/confirm | Session |  |
| POST | /api/v1/auth/2fa/disable | Session |  |
| POST | /api/v1/auth/2fa/recovery-codes/regenerate | Session |  |
| POST | /api/v1/auth/2fa/verify | Public |  |
| GET | /api/v1/auth/passkeys | Session |  |
| POST | /api/v1/auth/passkeys/register/begin | Session |  |
| POST | /api/v1/auth/passkeys/register/finish | Session |  |
| DELETE | /api/v1/auth/passkeys/{id} | Session |  |
| PATCH | /api/v1/auth/passkeys/{id} | Session |  |
| POST | /api/v1/auth/passkey-login/begin | Public |  |
| POST | /api/v1/auth/passkey-login/finish | Public |  |

</Accordion>

## Apps CRUD / Lifecycle / Deploy

<Accordion title="171 endpoints for app management, deployment, lifecycle control, and diagnostics">

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
| GET | /api/v1/apps/{name}/health | AbilityRead | handleGetAppHealth |
| PUT | /api/v1/apps/{name}/health | AbilityWrite | handleSetAppHealth |
| DELETE | /api/v1/apps/{name}/health | AbilityWrite | handleClearAppHealth |
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
| GET | /api/v1/deploys/failed | AbilityRead | handleListFailedDeploys |
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
| POST | /api/v1/apps/{name}/move-with-volumes | AbilityRoot | handleMoveAppWithVolumes |
| GET | /api/v1/apps/{name}/moves | AbilityRead | handleListAppVolumeMoves |
| GET | /api/v1/apps/{name}/moves/{id} | AbilityRead | handleGetAppVolumeMove |
| GET | /api/v1/apps/{name}/auto-rollback | AbilityRead | handleGetAutoRollback |
| PUT | /api/v1/apps/{name}/auto-rollback | AbilityDeploy | handleSetAutoRollback |
| GET | /api/v1/apps/{name}/auto-rollback-slo-burn | AbilityRead | handleGetAutoRollbackSLOBurn |
| PUT | /api/v1/apps/{name}/auto-rollback-slo-burn | AbilityDeploy | handleSetAutoRollbackSLOBurn |
| GET | /api/v1/apps/{name}/exec-access | AbilityRead | handleGetExecAccess |
| PUT | /api/v1/apps/{name}/exec-access | AbilityRoot | handleSetExecAccess |
| GET | /api/v1/apps/{name}/deploys/{deployId}/steps | AbilityRead | handleDeployStepStream |
| GET | /api/v1/apps/{name}/tags | AbilityRead | handleListAppTags |
| POST | /api/v1/apps/{name}/tags | AbilityWrite | handleAttachAppTag |
| DELETE | /api/v1/apps/{name}/tags/{id} | AbilityWrite | handleDetachAppTag |
| GET | /api/v1/apps/{name}/integrations | AbilityRead | handleListAppIntegrations |
| POST | /api/v1/apps/{name}/integrations | AbilityWriteSensitive | handleAttachAppIntegration |
| DELETE | /api/v1/apps/{name}/integrations/{id} | AbilityWriteSensitive | handleDetachAppIntegration |
| GET | /api/v1/apps/{name}/egress-policy | AbilityRead | handleGetAppEgressPolicy |
| PUT | /api/v1/apps/{name}/egress-policy | AbilityWriteSensitive | handleSetAppEgressPolicy |
| DELETE | /api/v1/apps/{name}/egress-policy | AbilityWriteSensitive | handleClearAppEgressPolicy |
| GET | /api/v1/apps/{name}/pipelines | AbilityRead | handleListPipelines |
| POST | /api/v1/apps/{name}/pipelines | AbilityWrite | handleCreatePipeline |
| GET | /api/v1/apps/{name}/pipelines/{pname} | AbilityRead | handleGetPipeline |
| PUT | /api/v1/apps/{name}/pipelines/{pname} | AbilityWrite | handleUpdatePipeline |
| DELETE | /api/v1/apps/{name}/pipelines/{pname} | AbilityWrite | handleDeletePipeline |
| POST | /api/v1/apps/{name}/pipelines/{pname}/runs | AbilityDeploy | handleStartPipelineRun |
| GET | /api/v1/apps/{name}/pipeline-runs | AbilityRead | handleListPipelineRuns |
| GET | /api/v1/apps/{name}/pipeline-runs/{id} | AbilityRead | handleGetPipelineRun |
| GET | /api/v1/apps/{name}/pipeline-runs/{id}/logs | AbilityRead | handleListPipelineRunLogs |
| GET | /api/v1/apps/{name}/pipeline-runs/{id}/logs/stream | AbilityRead | handleStreamPipelineRunLogs |
| POST | /api/v1/apps/{name}/pipeline-runs/{id}/cancel | AbilityDeploy | handleCancelPipelineRun |
| POST | /api/v1/apps/{name}/pipeline-runs/{id}/rerun | AbilityDeploy | handleRerunPipelineRun |
| POST | /api/v1/apps/{name}/pipeline-runs/{id}/approvals/{approval} | AbilityDeploy | handleDecidePipelineApproval |
| GET | /api/v1/apps/{name}/loadbalancer | AbilityRead | handleGetLoadBalancer |
| PUT | /api/v1/apps/{name}/loadbalancer | AbilityWrite | handleSetLoadBalancer |
| DELETE | /api/v1/apps/{name}/loadbalancer | AbilityWrite | handleDeleteLoadBalancer |
| POST | /api/v1/apps/{name}/loadbalancer/import | AbilityWrite | handleImportLoadBalancer |
| GET | /api/v1/apps/{name}/loadbalancer/status | AbilityRead | handleLoadBalancerStatus |
| GET | /api/v1/apps/{name}/loadbalancer/export | AbilityRead | handleExportLoadBalancer |
| POST | /api/v1/apps/{name}/pipeline-runs/{id}/hold | AbilityDeploy | handleDecidePipelineRunHold |
| GET | /api/v1/apps/{name}/pipeline-triggers | AbilityRead | handleListPipelineTriggers |
| GET | /api/v1/apps/{name}/pipeline-sync | AbilityRead | handleGetPipelineSync |
| PUT | /api/v1/apps/{name}/pipeline-sync | AbilityWrite | handleSetPipelineSync |
| POST | /api/v1/apps/{name}/pipeline-sync | AbilityWrite | handleRunPipelineSync |
| GET | /api/v1/apps/{name}/alert-history | AbilityRead | handleListAppAlertHistory |
| POST | /api/v1/apps/bulk | AbilityWrite | handleBulkApps |
| GET | /api/v1/apps/{name}/clone/preview | AbilityRead | handleClonePreview |
| GET | /api/v1/apps/{name}/deploy-freeze | AbilityRead | handleGetAppDeployFreeze |
| PUT | /api/v1/apps/{name}/deploy-freeze | AbilityDeploy | handlePutAppDeployFreeze |
| POST | /api/v1/apps/{name}/preflight | AbilityRead | handlePreflightApp |
| GET | /api/v1/apps/{name}/requests | AbilityRead | handleQueryRequests |
| GET | /api/v1/apps/{name}/loadbalancer/history | AbilityRead | handleLoadBalancerHistory |
| POST | /api/v1/apps/{name}/loadbalancer/check | AbilityWrite | handleLoadBalancerCheck |
| PUT | /api/v1/apps/{name}/loadbalancer/upstreams/{id} | AbilityWrite | handleSetLoadBalancerUpstream |
| GET | /api/v1/apps/{name}/timeline | AbilityRead | handleAppTimeline |
| GET | /api/v1/apps/{name}/pending-changes | AbilityRead | handlePendingChanges |
| POST | /api/v1/apps/{name}/apply-pending | AbilityDeploy | handleApplyPending |
| GET | /api/v1/apps/{name}/preview | AbilityRead | handleGetPreview |
| PUT | /api/v1/apps/{name}/preview | AbilityWrite | handlePutPreview |
| POST | /api/v1/apps/{name}/preview/capture | AbilityWrite | handleCapturePreview |
| POST | /api/v1/apps/{name}/preview/prune | AbilityWrite | handlePrunePreview |
| GET | /api/v1/apps/{name}/preview/history | AbilityRead | handleListPreviews |
| GET | /api/v1/apps/{name}/deployments/{id}/preview | AbilityRead | handleGetPreviewImage |
| GET | /api/v1/apps/{name}/changes | AbilityRead | handleAppChanges |
| GET | /api/v1/apps/{name}/slo-preview | AbilityRead | handleSLOPreview |
| GET | /api/v1/apps/{name}/supply-chain | AbilityRead | handleGetSupplyChain |
| PUT | /api/v1/apps/{name}/supply-chain | AbilityWrite | handlePutSupplyChain |
| POST | /api/v1/apps/{name}/supply-chain/override | AbilityWrite | handleSupplyChainOverride |
| GET | /api/v1/apps/{name}/deployments/{id}/sbom | AbilityRead | handleGetSBOM |
| GET | /api/v1/apps/{name}/deployments/{id}/vulnerabilities | AbilityRead | handleGetVulnerabilities |
| POST | /api/v1/apps/{name}/deployments/{id}/scan | AbilityWrite | handleScanDeployment |
| POST | /api/v1/apps/{name}/deploys/{deployId}/cancel | AbilityDeploy | handleCancelDeploy |
| POST | /api/v1/apps/{name}/deploys/{deployId}/rollback | AbilityDeploy | handleRollbackToDeploy |
| GET | /api/v1/apps/{name}/cancel-superseded | AbilityRead | handleGetCancelSuperseded |
| PUT | /api/v1/apps/{name}/cancel-superseded | AbilityDeploy | handleSetCancelSuperseded |
| GET | /api/v1/apps/{name}/preview-policy | AbilityRead | handleGetPreviewPolicy |
| PUT | /api/v1/apps/{name}/preview-policy | AbilityWriteSensitive | handleSetPreviewPolicy |
| GET | /api/v1/apps/{name}/deploys/{deployId} | AbilityRead | handleGetDeploy |
| GET | /api/v1/apps/{name}/schedule | AbilityRead | handleGetAppSchedule |
| PUT | /api/v1/apps/{name}/schedule | AbilityWriteSensitive | handleSetAppSchedule |
| GET | /api/v1/apps/{name}/schedule/history | AbilityRead | handleListAppScheduleHistory |
| POST | /api/v1/service-templates/{id}/deploy | AbilityDeploy | handleDeployServiceTemplateNow |
| GET | /api/v1/apps/{name}/connections | AbilityRead | handleListAppConnections |
| POST | /api/v1/apps/{name}/connections | AbilityWrite | handleCreateAppConnection |
| DELETE | /api/v1/apps/{name}/connections/{env_var} | AbilityWrite | handleDeleteAppConnection |
| GET | /api/v1/apps/{name}/connectable-databases | AbilityRead | handleListConnectableDatabases |
| POST | /api/v1/apps/{name}/validate-spec | AbilityRead | handleValidateSpec |
| GET | /api/v1/apps/{name}/health-score | AbilityRead | handleGetAppHealthScore |
| POST | /api/v1/apps/{name}/save-as-template | AbilityWrite | handleSaveAppAsTemplate |
| GET | /api/v1/apps/{name}/cost-estimate | AbilityRead | handleAppCostEstimate |
| GET | /api/v1/apps/{name}/badge | AbilityRead | handleGetBadgeSettings |
| PUT | /api/v1/apps/{name}/badge | AbilityRoot | handleSetBadgeSettings |
| GET | /api/v1/apps/{name}/badge.svg | Public | handlePublicAppBadge |
| GET | /api/v1/apps/{name}/deploys/{deployId}/probes | AbilityRead | handleListProbeAttempts |
| POST | /api/v1/apps/{name}/health/discover | AbilityRead | handleDiscoverAppHealth |
| GET | /api/v1/apps/{name}/streams | AbilityRead | handleListAppStreams |
| POST | /api/v1/apps/{name}/streams | AbilityWriteSensitive | handleCreateAppStream |
| DELETE | /api/v1/apps/{name}/streams/{id} | AbilityWriteSensitive | handleDeleteAppStream |
| GET | /api/v1/apps/git-sources | AbilityRead | handleListGitSources |
| GET | /api/v1/apps/{name}/auto-update | AbilityRead | handleGetImageAutoUpdate |
| PUT | /api/v1/apps/{name}/auto-update | AbilityDeploy | handleSetImageAutoUpdate |
| POST | /api/v1/apps/{name}/auto-update/check | AbilityDeploy | handleCheckImageAutoUpdate |
| POST | /api/v1/apps/{name}/auto-update/webhook | AbilityWriteSensitive | handleRotateImageUpdateWebhook |
| GET | /api/v1/apps/{name}/canary | AbilityRead | handleGetCanary |
| POST | /api/v1/apps/{name}/canary | AbilityDeploy | handleStartCanary |
| PUT | /api/v1/apps/{name}/canary | AbilityDeploy | handleSetCanaryWeight |
| DELETE | /api/v1/apps/{name}/canary | AbilityDeploy | handleAbortCanary |
| POST | /api/v1/apps/{name}/canary/promote | AbilityDeploy | handlePromoteCanary |
| GET | /api/v1/apps/{name}/sleep | AbilityRead | handleGetAppSleep |
| PUT | /api/v1/apps/{name}/sleep | AbilityDeploy | handleSetAppSleep |
| POST | /api/v1/apps/{name}/sleep/wake | AbilityDeploy | handleWakeApp |
| GET | /api/v1/apps/{name}/environment-domains | AbilityRead | handleGetAppEnvironmentDomains |
| GET | /api/v1/apps/{name}/listening-ports | AbilityRead | handleAppListeningPorts |
| GET | /api/v1/apps/{name}/compose | AbilityRead | handleGetComposeStack |
| GET | /api/v1/apps/{name}/compose/revisions | AbilityRead | handleListComposeRevisions |
| GET | /api/v1/apps/{name}/compose/revisions/{revision} | AbilityRead | handleGetComposeRevision |
| POST | /api/v1/apps/{name}/compose/rollback | AbilityDeploy | handleRollbackCompose |
| POST | /api/v1/apps/{name}/compose/redeploy | AbilityDeploy | handleRedeployCompose |
| POST | /api/v1/service-templates/{id}/install | AbilityDeploy | handleInstallServiceTemplate |
| DELETE | /api/v1/apps/{name}/compose | AbilityWrite | handleComposeDown |
| POST | /api/v1/apps/{name}/move | AbilityRoot | handleStartMove |
| GET | /api/v1/apps/{name}/placement | AbilityRead | handleGetPlacement |
| PUT | /api/v1/apps/{name}/placement | AbilityRoot | handleSetPlacement |
| GET | /api/v1/apps/{name}/investigate | AbilityRead | handleInvestigate |
| GET | /api/v1/apps/{name}/failure-context | AbilityRead | handleFailureContext |
| GET | /api/v1/apps/{name}/deploys/{deployId}/gate | AbilityRead | handleGetDeployGate |
| GET | /api/v1/apps/{name}/deploy-safety | AbilityRead | handleGetDeploySafety |
| PUT | /api/v1/apps/{name}/deploy-safety | AbilityDeploy | handlePutDeploySafety |
| GET | /api/v1/apps/{name}/rollback-targets | AbilityRead | handleListRollbackTargets |
| GET | /api/v1/apps/{name}/job-runs | AbilityRead | handleListScheduledTaskRuns |
| POST | /api/v1/apps/{name}/env/reveal | AbilityReadSensitive | handleRevealEnv |
| POST | /api/v1/apps/{name}/env/export | AbilityReadSensitive | handleExportEnvRevealed |
| GET | /api/v1/apps/{name}/env/export | AbilityRead | handleExportEnvRedacted |
| GET | /api/v1/apps/{name}/env/settings | AbilityRead | handleGetEnvSettings |
| PUT | /api/v1/apps/{name}/env/settings | AbilityWriteSensitive | handlePutEnvSettings |
| POST | /api/v1/apps/{name}/env/{key}/convert-to-secret | AbilityWriteSensitive | handleConvertEnvToSecret |
| GET | /api/v1/apps/{name}/secret-rotation | AbilityRead | handleListSecretRotation |
| GET | /api/v1/apps/{name}/secret-scan | AbilityRead | handleGetSecretScan |
| PUT | /api/v1/apps/{name}/secret-scan | AbilityWrite | handlePutSecretScan |
| POST | /api/v1/apps/{name}/secret-scan/run | AbilityWrite | handleRunSecretScan |
| POST | /api/v1/apps/{name}/secret-scan/allow | AbilityWrite | handleAllowSecretScan |
| DELETE | /api/v1/apps/{name}/secret-scan/allow/{fingerprint} | AbilityWrite | handleUnallowSecretScan |
| GET | /api/v1/apps/{name}/deployments/{id}/secret-scan | AbilityRead | handleGetDeploymentSecretScan |
| POST | /api/v1/apps/{name}/keep | AbilityWrite | handleKeepTrialApp |
| POST | /api/v1/apps/{name}/sleep/now | AbilityDeploy | handleSleepAppNow |
| GET | /api/v1/apps/{name}/tracing | AbilityRead | handleGetTracing |
| POST | /api/v1/apps/{name}/tracing | AbilityWriteSensitive | handleEnableTracing |
| DELETE | /api/v1/apps/{name}/tracing | AbilityWriteSensitive | handleDisableTracing |
| GET | /api/v1/apps/{name}/traces | AbilityRead | handleListTraces |
| GET | /api/v1/apps/{name}/traces/{id} | AbilityRead | handleGetTrace |
| GET | /api/v1/apps/{name}/log-pipeline | AbilityRead | handleGetLogPipeline |
| PUT | /api/v1/apps/{name}/log-pipeline | AbilityWriteSensitive | handleSetLogPipeline |
| DELETE | /api/v1/apps/{name}/log-pipeline | AbilityWriteSensitive | handleDeleteLogPipeline |
| POST | /api/v1/apps/{name}/log-pipeline/preview | AbilityRead | handlePreviewLogPipeline |
| GET | /api/v1/apps/{name}/access | AbilityRead | handleAppAccess |
| GET | /api/v1/apps/{name}/error-tracking | AbilityRead | handleGetErrorTracking |
| POST | /api/v1/apps/{name}/error-tracking | AbilityWriteSensitive | handleEnableErrorTracking |
| DELETE | /api/v1/apps/{name}/error-tracking | AbilityWriteSensitive | handleDisableErrorTracking |
| GET | /api/v1/apps/{name}/errors | AbilityRead | handleListErrors |
| GET | /api/v1/apps/{name}/errors/{fingerprint} | AbilityRead | handleGetError |
| PATCH | /api/v1/apps/{name}/errors/{fingerprint} | AbilityWriteSensitive | handlePatchError |
| GET | /api/v1/apps/{name}/log-budget | AbilityRead | handleGetLogBudget |
| PUT | /api/v1/apps/{name}/log-budget | AbilityWriteSensitive | handleSetLogBudget |
| DELETE | /api/v1/apps/{name}/log-budget | AbilityWriteSensitive | handleDeleteLogBudget |
| GET | /api/v1/apps/{name}/exposure | AbilityRead | handleGetAppExposure |
| POST | /api/v1/apps/{name}/exposure/preview | AbilityRead | handlePreviewAppExposure |
| PUT | /api/v1/apps/{name}/exposure | AbilityWriteSensitive | handleSetAppExposure |
| POST | /api/v1/apps/{name}/exposure/check | AbilityRead | handleCheckAppExposure |
| GET | /api/v1/apps/{name}/urls | AbilityRead | handleGetAppURLs |
| GET | /api/v1/apps/{name}/sleep/history | AbilityRead | handleAppSleepHistory |
| GET | /api/v1/apps/{name}/deploys/release-notes | AbilityRead | handleReleaseNotes |
| GET | /api/v1/apps/{name}/build-stats | AbilityRead | handleBuildStats |
| GET | /api/v1/apps/{name}/github-actions | AbilityRead | handleGetAppGitHubActions |
| PUT | /api/v1/apps/{name}/github-actions | AbilityWriteSensitive | handleSetAppGitHubActions |
| GET | /api/v1/apps/sleep/overview | AbilityRead | handleAppsSleepOverview |
| GET | /api/v1/apps/{name}/pipeline-runs/{id}/artifacts | AbilityRead | handleListPipelineRunArtifacts |
| GET | /api/v1/apps/{name}/pipeline-runs/{id}/artifacts/{aid}/download | AbilityRead | handleDownloadPipelineArtifact |
| GET | /api/v1/apps/{name}/pipelines/{pname}/webhook | AbilityRead | handleGetPipelineWebhook |
| PUT | /api/v1/apps/{name}/pipelines/{pname}/webhook | AbilityWriteSensitive | handleSetPipelineWebhook |
| DELETE | /api/v1/apps/{name}/pipelines/{pname}/webhook | AbilityWriteSensitive | handleDeletePipelineWebhook |
| POST | /api/v1/apps/{name}/pipelines/{pname}/webhook/rotate | AbilityWriteSensitive | handleRotatePipelineWebhook |
| GET | /api/v1/apps/{name}/pipelines/{pname}/webhook/deliveries | AbilityRead | handleListPipelineWebhookDeliveries |
| POST | /api/v1/apps/{name}/pipelines/{pname}/webhook/deliveries/{did}/replay | AbilityDeploy | handleReplayPipelineWebhookDelivery |
| GET | /api/v1/apps/{name}/maintenance | AbilityRead | handleGetAppMaintenance |
| PUT | /api/v1/apps/{name}/maintenance | AbilityDeploy | handleSetAppMaintenance |
| DELETE | /api/v1/apps/{name}/maintenance | AbilityDeploy | handleClearAppMaintenance |
| GET | /api/v1/apps/{name}/traffic | AbilityRead | handleAppTraffic |
| GET | /api/v1/apps/{name}/budget | AbilityRead | handleGetSpendCap |
| PUT | /api/v1/apps/{name}/budget | AbilityDeploy | handlePutSpendCap |
| DELETE | /api/v1/apps/{name}/budget | AbilityDeploy | handleDeleteSpendCap |
| GET | /api/v1/apps/{name}/budget/history | AbilityRead | handleSpendCapHistory |
| POST | /api/v1/apps/{name}/deploys/from-cache | AbilityDeploy | handleDeployFromCache |
| GET | /api/v1/apps/{name}/placement/decision | AbilityRead | handleGetPlacementDecision |
| GET | /api/v1/apps/{name}/autoscale | AbilityRead | handleGetAutoscale |
| PUT | /api/v1/apps/{name}/autoscale | AbilityDeploy | handlePutAutoscale |
| DELETE | /api/v1/apps/{name}/autoscale | AbilityDeploy | handleDeleteAutoscale |
| GET | /api/v1/apps/{name}/autoscale/history | AbilityRead | handleAutoscaleHistory |
| GET | /api/v1/apps/{name}/cache | AbilityRead | handleGetAppCache |
| POST | /api/v1/apps/{name}/cache/purge | AbilityDeploy | handlePurgeAppCache |
| GET | /api/v1/apps/{name}/cache/purges | AbilityRead | handleListCachePurges |
| POST | /api/v1/apps/{name}/cache/preset | AbilityDeploy | handleApplyCachePreset |
| POST | /api/v1/apps/{name}/env/scan | AbilityWrite | handleScanEnvValues |
| GET | /api/v1/apps/{name}/buckets | AbilityRead | handleListAppBuckets |
| PUT | /api/v1/apps/{name}/buckets/{id} | AbilityWriteSensitive | handleBindAppBucket |
| DELETE | /api/v1/apps/{name}/buckets/{id} | AbilityWriteSensitive | handleUnbindAppBucket |
| GET | /api/v1/apps/{name}/env/bound | AbilityRead | handleListBoundEnv |
| POST | /api/v1/apps/{name}/builds/{id}/cancel | AbilityDeploy | handleCancelBuild |
| POST | /api/v1/apps/{name}/builds/{id}/retry | AbilityDeploy | handleRetryBuild |
| GET | /api/v1/apps/{name}/pipeline-runs/{id}/fixes | AbilityRead | handleListPipelineRunFixes |
| POST | /api/v1/apps/{name}/pipeline-runs/{id}/fixes/apply | AbilityDeploy | handleApplyPipelineRunFix |
| GET | /api/v1/apps/{name}/pipelines/{pname}/insights | AbilityRead | handlePipelineInsights |
| GET | /api/v1/apps/{name}/build-speed | AbilityRead | handleBuildSpeed |
| GET | /api/v1/apps/{name}/ci/workflow | AbilityRead | handleGetCIWorkflow |
| GET | /api/v1/apps/{name}/ci/trusts | AbilityRead | handleListCITrusts |
| POST | /api/v1/apps/{name}/ci/trusts | AbilityWriteSensitive | handleCreateCITrust |
| DELETE | /api/v1/apps/{name}/ci/trusts/{id} | AbilityWriteSensitive | handleDeleteCITrust |
| GET | /api/v1/apps/{name}/env/check | AbilityRead | handleCheckEnv |
| POST | /api/v1/apps/{name}/env/check/fix | AbilityWrite | handleFixEnv |
| GET | /api/v1/apps/{name}/env/history | AbilityRead | handleListEnvHistory |
| POST | /api/v1/apps/{name}/log-budget/trim | AbilityWriteSensitive | handleTrimLogBudget |
| POST | /api/v1/apps/{name}/shield/recommended | AbilityDeploy | handleApplyRecommendedShield |
| GET | /api/v1/apps/{name}/ingress-limits | AbilityRead | handleGetAppIngressLimits |
| PUT | /api/v1/apps/{name}/ingress-limits | AbilityDeploy | handlePutAppIngressLimits |
| GET | /api/v1/apps/{name}/git-remote | AbilityRead | handleGetGitRemote |
| POST | /api/v1/apps/{name}/git-remote | AbilityWriteSensitive | handleIssueGitRemote |
| PUT | /api/v1/apps/{name}/git-remote | AbilityWriteSensitive | handleSetGitRemoteBranch |
| DELETE | /api/v1/apps/{name}/git-remote | AbilityWriteSensitive | handleDeleteGitRemote |
| GET | /api/v1/git/{repo}/info/refs | Public | handleGitInfoRefs |
| POST | /api/v1/git/{repo}/git-receive-pack | Public | handleGitReceivePack |
| GET | /api/v1/git/repos | AbilityReadSensitive | handleListGitRepos |
| GET | /api/v1/apps/{name}/watch | AbilityRead | handleGetAppWatch |
| POST | /api/v1/apps/{name}/watch/stop | AbilityDeploy | handleStopAppWatch |
| POST | /api/v1/apps/{name}/uploads | AbilityDeploy | handleUploadSource |
| GET | /api/v1/apps/{name}/uploads | AbilityRead | handleListUploads |
| GET | /api/v1/apps/{name}/build-limits | AbilityRead | handleGetAppBuildLimits |
| PUT | /api/v1/apps/{name}/build-limits | AbilityRoot | handleSetAppBuildLimits |
| DELETE | /api/v1/apps/{name}/build-limits | AbilityRoot | handleResetAppBuildLimits |
| GET | /api/v1/apps/{name}/go-live/checklist | AbilityRead | handleGoLiveChecklist |
| GET | /api/v1/apps/{name}/go-live/stream | AbilityRead | handleGoLiveChecklistStream |
| GET | /api/v1/apps/{name}/dns-provider | AbilityRead | handleGetAppDNSProvider |
| PUT | /api/v1/apps/{name}/dns-provider | AbilityWriteSensitive | handleSetAppDNSProvider |
| POST | /api/v1/apps/{name}/dns-provider/test | AbilityWriteSensitive | handleTestAppDNSProvider |
| GET | /api/v1/apps/{name}/build-hooks | AbilityRead | handleListBuildHooks |
| POST | /api/v1/apps/{name}/build-hooks | AbilityWriteSensitive | handleCreateBuildHook |
| POST | /api/v1/apps/{name}/build-hooks/{id}/rotate | AbilityWriteSensitive | handleRotateBuildHook |
| DELETE | /api/v1/apps/{name}/build-hooks/{id} | AbilityWriteSensitive | handleRevokeBuildHook |
| GET | /api/v1/apps/{name}/forms | AbilityRead | handleListForms |
| POST | /api/v1/apps/{name}/forms | AbilityDeploy | handleCreateForm |
| PUT | /api/v1/apps/{name}/forms/{form} | AbilityDeploy | handleUpdateForm |
| DELETE | /api/v1/apps/{name}/forms/{form} | AbilityDeploy | handleDeleteForm |
| GET | /api/v1/apps/{name}/forms/{form}/submissions | AbilityReadSensitive | handleListFormSubmissions |
| GET | /api/v1/apps/{name}/forms/{form}/export | AbilityReadSensitive | handleExportFormSubmissions |
| PUT | /api/v1/apps/{name}/forms/{form}/submissions/{id}/spam | AbilityDeploy | handleSetFormSubmissionSpam |
| DELETE | /api/v1/apps/{name}/forms/{form}/submissions/{id} | AbilityDeploy | handleDeleteFormSubmission |
| GET | /api/v1/apps/{name}/branch-deploys | AbilityRead | handleListBranchDeploys |
| PUT | /api/v1/apps/{name}/branch-deploys | AbilityWriteSensitive | handleSetBranchDeploySettings |
| DELETE | /api/v1/apps/{name}/branch-deploys/{slug} | AbilityDeploy | handleDeleteBranchDeploy |
| POST | /api/v1/apps/{name}/deploys/{deployId}/rollback-lock | AbilityDeploy | handleRollbackAndLock |
| POST | /api/v1/apps/{name}/deploys/{deployId}/publish | AbilityDeploy | handlePublishHeldBuild |
| GET | /api/v1/apps/{name}/publish-lock | AbilityRead | handleGetPublishLock |
| PUT | /api/v1/apps/{name}/publish-lock | AbilityDeploy | handlePutPublishLock |
| DELETE | /api/v1/apps/{name}/publish-lock | AbilityDeploy | handleDeletePublishLock |
| GET | /api/v1/apps/{name}/contexts | AbilityRead | handleListAppContexts |
| PUT | /api/v1/apps/{name}/contexts | AbilityWriteSensitive | handleSetAppContext |
| DELETE | /api/v1/apps/{name}/contexts | AbilityWrite | handleDeleteAppContext |
| GET | /api/v1/apps/{name}/effective-config | AbilityRead | handleGetEffectiveConfig |
| GET | /api/v1/apps/{name}/static-routes | AbilityRead | handleGetStaticRoutes |
| POST | /api/v1/apps/{name}/static-routes/test | AbilityRead | handleTestStaticRoutes |
| POST | /api/v1/apps/{name}/static-routes/lint | AbilityRead | handleLintStaticRoutes |
| GET | /api/v1/apps/{name}/split | AbilityRead | handleGetSplit |
| POST | /api/v1/apps/{name}/split | AbilityDeploy | handleStartSplit |
| PUT | /api/v1/apps/{name}/split | AbilityDeploy | handleSetSplitWeights |
| DELETE | /api/v1/apps/{name}/split | AbilityDeploy | handleStopSplit |
| POST | /api/v1/apps/{name}/split/promote | AbilityDeploy | handlePromoteSplit |
| GET | /api/v1/apps/{name}/forms/{form}/fields | AbilityRead | handleGetFormFields |
| PUT | /api/v1/apps/{name}/forms/{form}/fields | AbilityDeploy | handleSetFormFields |
| POST | /api/v1/apps/{name}/forms/{form}/test | AbilityDeploy | handleSendFormTest |
| POST | /api/v1/apps/{name}/budget/dry-run | AbilityRead | handleAppSpendDryRun |
| POST | /api/v1/apps/{name}/budget/override | AbilityRoot | handleSpendOverride |
| GET | /api/v1/apps/{name}/abuse | AbilityRead | handleGetAppAbuse |
| PUT | /api/v1/apps/{name}/abuse | AbilityWriteSensitive | handlePutAppAbuse |
| POST | /api/v1/apps/{name}/abuse/dry-run | AbilityReadSensitive | handleAbuseDryRun |
| POST | /api/v1/apps/{name}/scheduled-deploys | AbilityDeploy | handleCreateScheduledDeploy |
| GET | /api/v1/apps/{name}/scheduled-deploys | AbilityRead | handleListScheduledDeploys |
| GET | /api/v1/apps/{name}/scheduled-deploys/{id} | AbilityRead | handleGetScheduledDeploy |
| DELETE | /api/v1/apps/{name}/scheduled-deploys/{id} | AbilityDeploy | handleCancelScheduledDeploy |
| GET | /api/v1/apps/{name}/status | AbilityRead | handleAppStatus |
| GET | /api/v1/apps/{name}/files | AbilityRead | handleListAppFiles |
| PUT | /api/v1/apps/{name}/files | AbilityWriteSensitive | handleSetAppFile |
| DELETE | /api/v1/apps/{name}/files | AbilityWriteSensitive | handleDeleteAppFile |
| GET | /api/v1/apps/{name}/template-upgrade | AbilityRead | handleGetTemplateUpgrade |
| GET | /api/v1/apps/{name}/tunnel | AbilityRoot | handleAppTunnel |
| POST | /api/v1/apps/{name}/template-upgrade | AbilityDeploy | handleApplyTemplateUpgrade |
| GET | /api/v1/apps/{name}/template-upgrade/run | AbilityRead | handleGetTemplateUpgradeRun |

</Accordion>

`PUT /api/v1/apps/{name}/health` replaces only the app's health config (the body is a `ServiceHealth`: `readiness`, `liveness`, `ready_timeout`), leaving every other field alone; an empty body clears it. Probe fields and validation match [the app spec](/app-spec-reference#probe), with durations in nanoseconds.

## Secrets / Git Source / Webhooks / Preview Environments

Endpoints for:
- Encrypted secrets management and locking
- Git repository connections and configuration
- Webhook delivery history and replay
- [Preview environment](/previews) configuration and lifecycle

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
| DELETE | /api/v1/apps/{name}/secrets/{key} | AbilityWriteSensitive | handleDeleteSecret |
| PUT | /api/v1/apps/{name}/git-source/deploy-settings | AbilityWriteSensitive | handleSetGitDeploySettings |
| GET | /api/v1/previews | AbilityRead | handleListAllPreviews |
| POST | /api/v1/apps/{name}/previews/{number}/approve | AbilityWriteSensitive | handleApprovePreviewEnvironment |
| POST | /api/v1/apps/{name}/git-source/rotate-webhook-secret | AbilityWriteSensitive | handleRotateGitSourceWebhookSecret |
| POST | /api/v1/webhooks/slack/interactions | Public | handleSlackInteraction |
| POST | /api/v1/webhooks/discord/interactions | Public | handleDiscordInteraction |
| PUT | /api/v1/apps/{name}/git-source/build | AbilityWriteSensitive | handleSetGitSourceBuild |
| POST | /api/v1/apps/{name}/git-source/detect | AbilityDeploy | handleDetectGitSourceBuild |
| POST | /api/v1/apps/{name}/previews/{number}/extend | AbilityDeploy | handleExtendPreviewEnvironment |
| GET | /api/v1/apps/{name}/git-source/commits | AbilityDeploy | handleListGitSourceCommits |
| POST | /api/v1/apps/{name}/secrets/{key}/reveal | AbilityReadSensitive | handleRevealSecret |
| PUT | /api/v1/apps/{name}/secrets/{key}/rotation | AbilityWriteSensitive | handleSetSecretRotation |
| DELETE | /api/v1/apps/{name}/secrets/{key}/rotation | AbilityWriteSensitive | handleDeleteSecretRotation |
| POST | /api/v1/apps/{name}/secrets/{key}/rotation/rotate | AbilityWriteSensitive | handleRotateSecretNow |
| POST | /api/v1/apps/{name}/git-source/affected | AbilityRead | handleGitSourceAffected |
| GET | /api/v1/apps/{name}/previews/{number}/env | AbilityRead | handleListPRPreviewEnv |
| POST | /api/v1/apps/{name}/previews/{number}/env | AbilityWriteSensitive | handleSetPRPreviewEnv |
| DELETE | /api/v1/apps/{name}/previews/{number}/env/{id} | AbilityWrite | handleDeletePRPreviewEnv |
| POST | /api/v1/apps/{name}/git-source/validate | AbilityDeploy | handleValidateGitSourceAccess |
| POST | /api/v1/apps/{name}/git-source/webhook/register | AbilityWriteSensitive | handleRegisterGitSourceWebhook |
| POST | /api/v1/apps/{name}/secrets/{key}/rotate | AbilityWriteSensitive | handleRotateSecretValue |
| POST | /api/v1/webhooks/functions/{name} | Public | handleFunctionWebhook |
| POST | /api/v1/apps/{name}/secrets/{key}/build-time | AbilityWriteSensitive | handleSetSecretBuildTime |

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
| GET | /api/v1/apps/{name}/logs/signatures | AbilityRead | handleLogSignatures |

## Alerts / Scheduled Tasks / Feature Flags / Notification Channels

Endpoints for:
- Alert rule creation and management
- Scheduled task configuration and execution
- [Feature flag](/feature-flags) configuration and evaluation
- Notification channel management and delivery tracking

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| POST | /api/v1/apps/{name}/alerts | AbilityWrite | handleCreateAlertRule |
| GET | /api/v1/apps/{name}/alerts | AbilityRead | handleListAlertRules |
| PUT | /api/v1/apps/{name}/alerts/{id} | AbilityWrite | handleUpdateAlertRule |
| DELETE | /api/v1/apps/{name}/alerts/{id} | AbilityWrite | handleDeleteAlertRule |
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
| POST | /api/v1/apps/{name}/alerts/{id}/silence | AbilityWrite | handleSilenceAlertRule |
| POST | /api/v1/apps/{name}/alerts/apply-template | AbilityWrite | handleApplyAlertTemplate |
| GET | /api/v1/notification-channels/{id}/quiet-hours | AbilityRead | handleGetChannelQuietHours |
| PUT | /api/v1/notification-channels/{id}/quiet-hours | AbilityWrite | handlePutChannelQuietHours |
| DELETE | /api/v1/notification-channels/{id}/quiet-hours | AbilityWrite | handleDeleteChannelQuietHours |

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
| GET | /api/v1/databases/{name}/slow-queries | AbilityRead | handleQueryDatabaseSlowQueries |
| POST | /api/v1/databases/{name}/pitr | AbilityWriteSensitive | handleEnablePITR |
| DELETE | /api/v1/databases/{name}/pitr | AbilityWriteSensitive | handleDisablePITR |
| GET | /api/v1/databases/{name}/pitr | AbilityRead | handleGetPITRStatus |
| POST | /api/v1/databases/{name}/base-backups | AbilityWriteSensitive | handleTriggerBaseBackup |
| GET | /api/v1/databases/{name}/base-backups | AbilityRead | handleListBaseBackupHistory |
| POST | /api/v1/databases/{name}/pitr-restore | AbilityRoot | handleTriggerPITRRestore |
| GET | /api/v1/databases/{name}/pitr-restores | AbilityRead | handleListPITRRestoreHistory |
| PUT | /api/v1/databases/{name}/version | AbilityWriteSensitive | handleSetDatabaseVersion |
| POST | /api/v1/databases/{name}/major-upgrade | AbilityRoot | handleMajorUpgrade |
| GET | /api/v1/databases/{name}/major-upgrades | AbilityRead | handleListMajorUpgrades |
| POST | /api/v1/databases/{name}/major-upgrades/{id}/rollback | AbilityRoot | handleRollbackMajorUpgrade |
| DELETE | /api/v1/databases/{name}/major-upgrades/{id}/snapshot | AbilityRoot | handleDiscardMajorUpgradeSnapshot |
| PUT | /api/v1/databases/{name}/environment | AbilityWrite | handleSetDatabaseEnvironment |
| GET | /api/v1/databases/{name}/schema | AbilityReadSensitive | handleGetDatabaseSchema |
| GET | /api/v1/databases/{name}/tables/{schema}/{table}/rows | AbilityReadSensitive | handleGetDatabaseTableRows |
| POST | /api/v1/databases/{name}/query | AbilityReadSensitive | handleDatabaseQuery |
| POST | /api/v1/databases/{name}/query/write | AbilityRoot | handleDatabaseQueryWrite |
| POST | /api/v1/databases/{name}/explain | AbilityReadSensitive | handleDatabaseExplain |
| GET | /api/v1/databases/{name}/keys | AbilityReadSensitive | handleScanDatabaseKeys |
| GET | /api/v1/databases/{name}/key | AbilityReadSensitive | handleGetDatabaseKey |
| GET | /api/v1/databases/{name}/query-history | AbilityReadSensitive | handleListDatabaseQueryHistory |
| DELETE | /api/v1/databases/{name}/query-history | AbilityReadSensitive | handleClearDatabaseQueryHistory |
| GET | /api/v1/databases/{name}/saved-queries | AbilityReadSensitive | handleListDatabaseSavedQueries |
| POST | /api/v1/databases/{name}/saved-queries | AbilityReadSensitive | handleSaveDatabaseQuery |
| DELETE | /api/v1/databases/{name}/saved-queries/{id} | AbilityReadSensitive | handleDeleteDatabaseSavedQuery |
| GET | /api/v1/databases/{name}/users | AbilityReadSensitive | handleListDatabaseUsers |
| POST | /api/v1/databases/{name}/users | AbilityRoot | handleCreateDatabaseUser |
| POST | /api/v1/databases/{name}/users/{role}/rotate | AbilityRoot | handleRotateDatabaseUser |
| POST | /api/v1/databases/{name}/users/{role}/disable | AbilityRoot | handleDisableDatabaseUser |
| POST | /api/v1/databases/{name}/users/{role}/enable | AbilityRoot | handleEnableDatabaseUser |
| DELETE | /api/v1/databases/{name}/users/{role} | AbilityRoot | handleDeleteDatabaseUser |
| GET | /api/v1/databases/{name}/access/temp | AbilityReadSensitive | handleListDatabaseTempCredentials |
| POST | /api/v1/databases/{name}/access/temp | AbilityRoot | handleIssueDatabaseTempCredential |
| DELETE | /api/v1/databases/{name}/access/temp/{id} | AbilityRoot | handleRevokeDatabaseTempCredential |
| GET | /api/v1/databases/{name}/access/principals | AbilityReadSensitive | handleDatabaseWhoCanAccess |
| POST | /api/v1/databases/{name}/access/grants | AbilityRoot | handleGrantDatabaseAccess |
| DELETE | /api/v1/databases/{name}/access/grants/{policy_id}/{principal_type}/{principal_id} | AbilityRoot | handleRevokeDatabaseGrant |
| GET | /api/v1/databases/{name}/network | AbilityRead | handleGetDatabaseNetwork |
| POST | /api/v1/databases/{name}/network/rules/preview | AbilityRoot | handlePreviewDatabaseRules |
| PUT | /api/v1/databases/{name}/network/rules | AbilityRoot | handleApplyDatabaseRules |
| DELETE | /api/v1/databases/{name}/network/rules | AbilityRoot | handleRemoveDatabaseRules |
| POST | /api/v1/databases/{name}/network/make-private | AbilityRoot | handleMakeDatabasePrivate |
| PUT | /api/v1/databases/{name}/network/scope | AbilityRoot | handleSetDatabaseScope |
| PUT | /api/v1/databases/{name}/network/tls | AbilityRoot | handleSetDatabaseTLS |
| GET | /api/v1/databases/{name}/tables/{schema}/{table}/structure | AbilityReadSensitive | handleGetDatabaseTableStructure |
| GET | /api/v1/databases/upgrade-summary | AbilityRead | handleDatabaseUpgradeSummary |
| GET | /api/v1/databases/{name}/upgrades | AbilityRead | handleGetDatabaseUpgrades |
| PUT | /api/v1/databases/{name}/upgrade-policy | AbilityWriteSensitive | handlePutDatabaseUpgradePolicy |
| POST | /api/v1/databases/{name}/upgrade-now | AbilityWriteSensitive | handleDatabaseUpgradeNow |
| POST | /api/v1/databases/{name}/move | AbilityRoot | handleStartMove |
| GET | /api/v1/databases/{name}/placement | AbilityRead | handleGetPlacement |
| PUT | /api/v1/databases/{name}/placement | AbilityRoot | handleSetPlacement |
| GET | /api/v1/databases/{name}/engine-stats | AbilityReadSensitive | handleGetDatabaseEngineStats |
| GET | /api/v1/databases/{name}/placement/decision | AbilityRead | handleGetPlacementDecision |
| GET | /api/v1/databases/{name}/links | AbilityRead | handleListDatabaseLinks |
| POST | /api/v1/databases/{name}/links | AbilityWrite | handleConnectDatabaseApp |
| GET | /api/v1/databases/{name}/password-rotation | AbilityReadSensitive | handleGetDatabasePasswordRotation |
| POST | /api/v1/databases/{name}/password-rotation | AbilityWriteSensitive | handleRotateDatabasePassword |
| PUT | /api/v1/databases/{name}/password-rotation/policy | AbilityRoot | handleSetDatabasePasswordPolicy |
| PUT | /api/v1/databases/{name}/tls | AbilityRoot | handleSetDatabaseTLSEnabled |
| GET | /api/v1/databases/{name}/tunnel | AbilityRoot | handleDatabaseTunnel |
| GET | /api/v1/databases/{name}/provision | AbilityRead | handleGetDatabaseProvision |
| GET | /api/v1/databases/{name}/provision/stream | AbilityRead | handleDatabaseProvisionStream |
| POST | /api/v1/databases/{name}/provision/retry | AbilityWrite | handleRetryDatabaseProvision |
| POST | /api/v1/databases/{name}/provision/discard | AbilityWrite | handleDiscardPendingDatabase |
| PUT | /api/v1/databases/{name}/pending-settings | AbilityWrite | handleUpdatePendingDatabase |
| POST | /api/v1/databases/{name}/credentials/reveal | AbilityRoot | handleRevealDatabaseCredentials |

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
| GET | /api/v1/environments/{id}/clone/preview | AbilityRead | handleEnvironmentClonePreview |
| POST | /api/v1/environments/{id}/clone | AbilityDeploy | handleEnvironmentClone |
| GET | /api/v1/environments/{id}/env | AbilityRead | handleGetEnvironmentEnv |
| PUT | /api/v1/environments/{id}/env | AbilityWrite | handleSetEnvironmentEnv |
| GET | /api/v1/projects/{id}/env | AbilityRead | handleGetProjectEnv |
| PUT | /api/v1/projects/{id}/env | AbilityWrite | handleSetProjectEnv |
| PUT | /api/v1/apps/{name}/project | AbilityWrite | handleSetAppProject |
| PUT | /api/v1/databases/{name}/project | AbilityWrite | handleSetDatabaseProject |
| PUT | /api/v1/databases/{name}/resources | AbilityWrite | handleSetDatabaseResources |
| GET | /api/v1/organizations/{id}/env/all | AbilityRead | handleListOrganizationEnvAll |
| GET | /api/v1/organizations/{id}/env/secrets | AbilityRead | handleListOrganizationEnvSecretKeys |
| PUT | /api/v1/organizations/{id}/env/secrets/{key} | AbilityWrite | handleSetOrganizationEnvSecret |
| DELETE | /api/v1/organizations/{id}/env/secrets/{key} | AbilityWrite | handleDeleteOrganizationEnvSecret |
| GET | /api/v1/environments/{id}/env/all | AbilityRead | handleListEnvironmentEnvAll |
| GET | /api/v1/environments/{id}/env/secrets | AbilityRead | handleListEnvironmentEnvSecretKeys |
| PUT | /api/v1/environments/{id}/env/secrets/{key} | AbilityWrite | handleSetEnvironmentEnvSecret |
| DELETE | /api/v1/environments/{id}/env/secrets/{key} | AbilityWrite | handleDeleteEnvironmentEnvSecret |
| GET | /api/v1/projects/{id}/env/all | AbilityRead | handleListProjectEnvAll |
| GET | /api/v1/projects/{id}/env/secrets | AbilityRead | handleListProjectEnvSecretKeys |
| PUT | /api/v1/projects/{id}/env/secrets/{key} | AbilityWrite | handleSetProjectEnvSecret |
| DELETE | /api/v1/projects/{id}/env/secrets/{key} | AbilityWrite | handleDeleteProjectEnvSecret |
| GET | /api/v1/projects/{id}/environments/compare | AbilityRead | handleCompareEnvironmentEnv |
| GET | /api/v1/projects/{id}/topology | AbilityRead | handleGetProjectTopology |
| GET | /api/v1/environments | AbilityRead | handleListAllEnvironments |
| POST | /api/v1/environments | AbilityWrite | handleCreateGlobalEnvironment |
| GET | /api/v1/projects/{id}/quota | AbilityRead | handleGetQuota |
| PUT | /api/v1/projects/{id}/quota | AbilityWrite | handleSetQuota |
| DELETE | /api/v1/projects/{id}/quota | AbilityWrite | handleDeleteQuota |

## Nodes

Endpoints for:
- Physical infrastructure management and monitoring
- Node health and status
- Workload assignment and placement
- Cordon, drain, and lifecycle operations
- Node-level metrics and patch status
- Agent certificate re-enrollment and revocation

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
| GET | /api/v1/nodes/{id}/events | AbilityRoot | handleListNodeEvents |
| GET | /api/v1/nodes/{id}/capacity-forecast | AbilityRoot | handleNodeCapacityForecast |
| POST | /api/v1/nodes/{id}/reenroll-token | AbilityRoot | handleCreateNodeReenrollToken |
| POST | /api/v1/nodes/{id}/revoke-cert | AbilityRoot | handleRevokeNodeCert |
| POST | /api/v1/nodes/{id}/mesh/rotate-key | AbilityRoot | handleRotateNodeMeshKey |
| GET | /api/v1/nodes/resource-usage | AbilityRoot | handleFleetResourceUsage |
| POST | /api/v1/nodes/provision | AbilityRoot | handleCreateNodeProvision |
| PUT | /api/v1/nodes/{id}/region | AbilityRoot | handleSetNodeRegion |
| POST | /api/v1/nodes/ssh-provision | AbilityRoot | handleCreateSSHNodeProvision |
| POST | /api/v1/nodes/{id}/mesh/rejoin | AbilityRoot | handleRejoinNodeMesh |
| GET | /api/v1/nodes/{id}/drains | AbilityRoot | handleListNodeDrains |
| GET | /api/v1/nodes/{id}/labels | AbilityRoot | handleGetNodeLabels |
| PUT | /api/v1/nodes/{id}/labels | AbilityRoot | handleSetNodeLabels |
| POST | /api/v1/nodes/ssh-provision/preflight | AbilityRoot | handleSSHProvisionPreflight |
| GET | /api/v1/nodes/{id}/security | AbilityRoot | handleGetNodeTrust |
| GET | /api/v1/nodes/{id}/security/events | AbilityRoot | handleListNodeSecurityEvents |
| POST | /api/v1/nodes/{id}/cert/renew | AbilityRoot | handleRenewNodeCert |
| GET | /api/v1/nodes/allocations | AbilityRoot | handleNodeAllocations |
| GET | /api/v1/nodes/zones | AbilityRoot | handleListZones |
| GET | /api/v1/nodes/join-tokens | AbilityRoot | handleListPendingNodeJoinTokens |
| DELETE | /api/v1/nodes/join-tokens | AbilityRoot | handleRevokeNodeJoinTokens |
| DELETE | /api/v1/nodes/join-tokens/{id} | AbilityRoot | handleRevokeNodeJoinToken |
| GET | /api/v1/nodes/placement-simulation | AbilityRoot | handlePlacementSimulation |
| GET | /api/v1/nodes/failure-simulation | AbilityRoot | handleFailureSimulation |
| POST | /api/v1/nodes/{id}/cert/drill | AbilityRoot | handleNodeCertDrill |
| GET | /api/v1/nodes/{id}/capacity | AbilityRoot | handleGetNodeCapacity |
| PUT | /api/v1/nodes/{id}/guardrails | AbilityRoot | handlePutNodeGuardrails |
| POST | /api/v1/nodes/{id}/cleanup | AbilityRoot | handleNodeCleanup |
| GET | /api/v1/nodes/{id}/cleanup-runs | AbilityRoot | handleListNodeCleanupRuns |
| GET | /api/v1/nodes/{id}/failure | AbilityRoot | handleGetNodeFailure |
| PUT | /api/v1/nodes/{id}/failure-policy | AbilityRoot | handleSetNodeFailurePolicy |
| POST | /api/v1/nodes/{id}/failure/lost | AbilityRoot | handleMarkNodeLost |
| POST | /api/v1/nodes/{id}/failure/resolve | AbilityRoot | handleResolveNodeFailure |
| GET | /api/v1/nodes/domains | AbilityRoot | handleNodeDomains |
| GET | /api/v1/nodes/{id}/onboarding | AbilityRoot | handleGetNodeOnboarding |

## Ingress / Certificates / Domains / Email / Cloudflare

<Accordion title="91 endpoints for TLS, domains, ingress control, and DNS/Vault integrations">

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
| GET | /api/v1/settings/dashboard-url | AbilityRead | handleGetDashboardURL |
| PUT | /api/v1/settings/dashboard-url | AbilityRoot | handleUpdateDashboardURL |
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
| GET | /api/v1/apps/{name}/branch-env | AbilityRead | handleListAppBranchEnv |
| POST | /api/v1/apps/{name}/branch-env | AbilityWriteSensitive | handleSetAppBranchEnv |
| DELETE | /api/v1/apps/{name}/branch-env/{id} | AbilityWrite | handleDeleteAppBranchEnv |
| PATCH | /api/v1/apps/{name}/domains | AbilityWrite | handleEditAppDomains |
| POST | /api/v1/apps/{name}/domains/{domain}/cert/renew | AbilityRoot | handleRenewDomainCertificate |
| GET | /api/v1/apps/{name}/domains/{domain}/dns-records | AbilityRead | handleListDNSRecords |
| POST | /api/v1/apps/{name}/domains/{domain}/dns-records | AbilityRoot | handleCreateDNSRecord |
| PUT | /api/v1/apps/{name}/domains/{domain}/dns-records | AbilityRoot | handleUpdateDNSRecord |
| DELETE | /api/v1/apps/{name}/domains/{domain}/dns-records | AbilityRoot | handleDeleteDNSRecord |
| POST | /api/v1/settings/email/test | AbilityRoot | handleTestEmail |
| GET | /api/v1/settings/ingress/https | AbilityRead | handleGetHTTPSStatus |
| POST | /api/v1/settings/ingress/https | AbilityRoot | handleEnableHTTPS |
| GET | /api/v1/apps/{name}/domains/{domain}/search-visibility | AbilityRead | handleGetDomainSearchVisibility |
| PUT | /api/v1/apps/{name}/domains/{domain}/search-visibility | AbilityDeploy | handleSetDomainSearchVisibility |
| GET | /api/v1/apps/{name}/domains/{domain}/headers | AbilityRead | handleGetDomainPolicy |
| PUT | /api/v1/apps/{name}/domains/{domain}/headers | AbilityDeploy | handleSetDomainPolicy |
| DELETE | /api/v1/apps/{name}/domains/{domain}/headers | AbilityDeploy | handleDeleteDomainPolicy |
| GET | /api/v1/apps/{name}/domains/{domain}/forwarders | AbilityRead | handleGetDomainPolicy |
| PUT | /api/v1/apps/{name}/domains/{domain}/forwarders | AbilityDeploy | handleSetDomainPolicy |
| DELETE | /api/v1/apps/{name}/domains/{domain}/forwarders | AbilityDeploy | handleDeleteDomainPolicy |
| GET | /api/v1/apps/{name}/domains/{domain}/geo | AbilityRead | handleGetDomainPolicy |
| PUT | /api/v1/apps/{name}/domains/{domain}/geo | AbilityDeploy | handleSetDomainPolicy |
| DELETE | /api/v1/apps/{name}/domains/{domain}/geo | AbilityDeploy | handleDeleteDomainPolicy |
| GET | /api/v1/apps/{name}/domains/{domain}/cache | AbilityRead | handleGetDomainPolicy |
| PUT | /api/v1/apps/{name}/domains/{domain}/cache | AbilityDeploy | handleSetDomainPolicy |
| DELETE | /api/v1/apps/{name}/domains/{domain}/cache | AbilityDeploy | handleDeleteDomainPolicy |
| POST | /api/v1/apps/{name}/domains/{domain}/cache/purge | AbilityDeploy | handlePurgeDomainCache |
| GET | /api/v1/apps/{name}/domains/{domain}/cache/stats | AbilityRead | handleGetDomainCacheStats |
| GET | /api/v1/apps/{name}/domains/{domain}/policies | AbilityRead | handleGetDomainPolicies |
| POST | /api/v1/apps/{name}/domains/{domain}/policies/preview | AbilityRead | handlePreviewDomainPolicies |
| GET | /api/v1/apps/{name}/domains/{domain}/redirects | AbilityRead | handleGetDomainRedirects |
| PUT | /api/v1/apps/{name}/domains/{domain}/redirects | AbilityDeploy | handleSetDomainRedirectSettings |
| DELETE | /api/v1/apps/{name}/domains/{domain}/redirects | AbilityDeploy | handleDeleteDomainRedirectSettings |
| POST | /api/v1/apps/{name}/domains/{domain}/redirects/canonical | AbilityDeploy | handleSetDomainCanonical |
| PUT | /api/v1/apps/{name}/domains/{domain}/redirects/aliases | AbilityDeploy | handleSetDomainAliases |
| GET | /api/v1/apps/{name}/domains/{domain}/ports | AbilityRead | handleGetDomainPorts |
| PUT | /api/v1/apps/{name}/domains/{domain}/ports/{port}/restrict | AbilityWriteSensitive | handleRestrictDomainPort |
| DELETE | /api/v1/apps/{name}/domains/{domain}/ports/{port}/restrict | AbilityWriteSensitive | handleUnrestrictDomainPort |
| GET | /api/v1/apps/{name}/domains/{domain}/go-live | AbilityRead | handleGoLiveStatus |
| POST | /api/v1/apps/{name}/domains/{domain}/go-live | AbilityRoot | handleGoLiveRun |
| POST | /api/v1/apps/{name}/domains/go-live/plan | AbilityWrite | handleGoLivePlan |
| POST | /api/v1/settings/ingress/apps-base-domain/backfill | AbilityRoot | handleBackfillBaseDomain |
| POST | /api/v1/apps/{name}/domains/{domain}/doctor | AbilityRead | handleDomainDoctor |
| GET | /api/v1/apps/{name}/domains/{domain}/auth-gate | AbilityRead | handleGetDomainAuthGate |
| PUT | /api/v1/apps/{name}/domains/{domain}/auth-gate | AbilityRoot | handleSetDomainAuthGate |
| DELETE | /api/v1/apps/{name}/domains/{domain}/auth-gate | AbilityRoot | handleClearDomainAuthGate |
| GET | /api/v1/apps/{name}/domains/{domain}/auth-gate/events | AbilityReadSensitive | handleListAuthGateEvents |
| DELETE | /api/v1/apps/{name}/domains/{domain}/auth-gate/sessions | AbilityRoot | handleRevokeAuthGateSessions |
| GET | /api/v1/apps/{name}/domains/{domain}/shield | AbilityRead | handleGetDomainPolicy |
| PUT | /api/v1/apps/{name}/domains/{domain}/shield | AbilityDeploy | handleSetDomainPolicy |
| DELETE | /api/v1/apps/{name}/domains/{domain}/shield | AbilityDeploy | handleDeleteDomainPolicy |
| POST | /api/v1/apps/{name}/domains/{domain}/shield/under-attack | AbilityDeploy | handleStartUnderAttack |
| DELETE | /api/v1/apps/{name}/domains/{domain}/shield/under-attack | AbilityDeploy | handleStopUnderAttack |
| GET | /api/v1/settings/ingress/edge | AbilityRead | handleGetIngressEdge |
| PUT | /api/v1/settings/ingress/edge | AbilityRoot | handlePutIngressEdge |
| POST | /api/v1/settings/ingress/edge/trusted-proxies/apply-detected | AbilityRoot | handleApplyDetectedProxy |
| GET | /api/v1/settings/email/status | AbilityRead | handleEmailStatus |
| GET | /api/v1/settings/email/domain-check | AbilityRoot | handleEmailDomainCheck |
| GET | /api/v1/settings/email/deliveries | AbilityRoot | handleListEmailDeliveries |
| GET | /api/v1/settings/email/deliveries/{id} | AbilityRoot | handleGetEmailDelivery |
| POST | /api/v1/settings/vault/rotate-credential | AbilityRoot | handleRotateVaultCredential |
| GET | /api/v1/settings/email/providers | AbilityRead | handleListEmailProviders |
| POST | /api/v1/settings/email/providers | AbilityRoot | handleCreateEmailProvider |
| GET | /api/v1/settings/email/providers/{id} | AbilityRead | handleGetEmailProvider |
| PUT | /api/v1/settings/email/providers/{id} | AbilityRoot | handleUpdateEmailProvider |
| DELETE | /api/v1/settings/email/providers/{id} | AbilityRoot | handleDeleteEmailProvider |
| POST | /api/v1/settings/email/providers/{id}/test | AbilityRoot | handleTestEmailProvider |
| POST | /api/v1/settings/email/providers/{id}/promote | AbilityRoot | handlePromoteEmailProvider |
| POST | /api/v1/settings/email/providers/{id}/secrets/{field}/reveal | AbilityRoot | handleRevealEmailSecret |
| GET | /api/v1/settings/email/branding | AbilityRead | handleGetEmailBranding |
| PUT | /api/v1/settings/email/branding | AbilityRoot | handleUpdateEmailBranding |
| POST | /api/v1/settings/email/branding/preview | AbilityRoot | handlePreviewEmailBranding |

</Accordion>

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
| GET | /api/v1/domains/automation/runs | AbilityRead | handleListAutomationRuns |
| POST | /api/v1/domains/automation/runs/{id}/undo | AbilityRoot | handleUndoAutomationRun |
| GET | /api/v1/domains/{domain}/activity | AbilityRead | handleDomainActivity |
| GET | /api/v1/domains/standalone | AbilityRead | handleListStandaloneDomains |
| POST | /api/v1/domains/standalone | AbilityWriteSensitive | handleCreateStandaloneDomain |
| POST | /api/v1/domains/standalone/validate | AbilityWriteSensitive | handleValidateStandaloneDomain |
| GET | /api/v1/domains/standalone/candidates | AbilityWriteSensitive | handleStandaloneCandidates |
| PUT | /api/v1/domains/standalone/{domain} | AbilityWriteSensitive | handleUpdateStandaloneDomain |
| DELETE | /api/v1/domains/standalone/{domain} | AbilityWriteSensitive | handleDeleteStandaloneDomain |
| GET | /api/v1/registry-credentials/{id}/usage | AbilityRead | handleRegistryCredentialUsage |
| POST | /api/v1/registry-credentials/test | AbilityWriteSensitive | handleProbeRegistryCredential |
| DELETE | /api/v1/static-sites/{name} | AbilityWriteSensitive | handleDeleteStaticSite |
| POST | /api/v1/domains/{domain}/diagnose | AbilityRead | handleDomainDiagnose |
| GET | /api/v1/domains/{domain}/diagnose/stream | AbilityRead | handleDomainDiagnoseStream |
| GET | /api/v1/domains/{domain}/cert/preflight | AbilityRead | handleCertPreflight |
| GET | /api/v1/domains/{domain}/cert/status | AbilityRead | handleCertStatus |
| POST | /api/v1/domains/{domain}/cert | AbilityRoot | handleGetCert |
| GET | /api/v1/backup-targets/{id}/encryption | AbilityRead | handleGetBackupTargetEncryption |
| PUT | /api/v1/backup-targets/{id}/encryption | AbilityRoot | handleSetBackupTargetEncryption |
| POST | /api/v1/registry-credentials/presets/test | AbilityWriteSensitive | handleTestRegistryPreset |
| POST | /api/v1/registry-credentials/{id}/refresh | AbilityWriteSensitive | handleRefreshRegistryCredential |

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
| GET | /api/v1/dockerhub/search | AbilityRead | handleDockerHubSearch |
| GET | /api/v1/dockerhub/repositories/{namespace}/{repo}/tags | AbilityRead | handleDockerHubTags |

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
| GET | /api/v1/gitlab-app/connect | AbilityRoot | handleStartGitConnect |
| GET | /api/v1/gitlab-app/callback | AbilityRoot | handleGitConnectCallback |
| GET | /api/v1/bitbucket-app/connect | AbilityRoot | handleStartGitConnect |
| GET | /api/v1/bitbucket-app/callback | AbilityRoot | handleGitConnectCallback |
| GET | /api/v1/github-app/installations | AbilityRoot | handleListGitHubAppInstallations |
| DELETE | /api/v1/github-app/installations/{id} | AbilityRoot | handleDeleteGitHubAppInstallation |
| POST | /api/v1/git-providers/{provider}/validate | AbilityRoot | handleValidateGitProvider |

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
| DELETE | /api/v1/databases/{name}/backups/{historyId} | AbilityWriteSensitive | handleDeleteBackup |
| GET | /api/v1/backups/health | AbilityRead | handleBackupHealth |
| GET | /api/v1/backups/drills | AbilityRead | handleListBackupDrills |
| GET | /api/v1/backups/drills/{id} | AbilityRead | handleGetBackupDrill |
| POST | /api/v1/backups/drills | AbilityWriteSensitive | handleStartBackupDrill |
| POST | /api/v1/backups/protection/refresh | AbilityWriteSensitive | handleRefreshBackupProtection |
| GET | /api/v1/backups/coverage | AbilityRead | handleBackupCoverage |
| POST | /api/v1/backups/protect-all | AbilityWriteSensitive | handleProtectAll |
| POST | /api/v1/backups/encryption-key/ack | AbilityWriteSensitive | handleAckBackupEncryptionKey |
| GET | /api/v1/backups/copy-settings | AbilityRead | handleGetBackupCopySettings |
| PUT | /api/v1/backups/copy-settings | AbilityWriteSensitive | handleSetBackupCopySettings |
| GET | /api/v1/backups/copies | AbilityRead | handleListBackupCopies |

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
| DELETE | /api/v1/apps/{name}/volumes/{volume}/backups/{historyId} | AbilityWriteSensitive | handleDeleteVolumeBackup |
| PUT | /api/v1/apps/{name}/volumes | AbilityWrite | handleSetAppVolumes |
| GET | /api/v1/apps/{name}/volumes/{volume}/backup-policy | AbilityRead | handleGetVolumeBackupPolicy |
| PUT | /api/v1/apps/{name}/volumes/{volume}/backup-policy | AbilityWriteSensitive | handleSetVolumeBackupPolicy |
| POST | /api/v1/apps/{name}/volumes/{volume}/restore-to | AbilityWriteSensitive | handleVolumeRestoreTo |
| GET | /api/v1/apps/{name}/volumes/{volume}/files | AbilityReadSensitive | handleListVolumeFiles |
| GET | /api/v1/apps/{name}/volumes/{volume}/files/content | AbilityReadSensitive | handleDownloadVolumeFile |
| PUT | /api/v1/apps/{name}/volumes/{volume}/files/content | AbilityWrite | handlePutVolumeFile |
| DELETE | /api/v1/apps/{name}/volumes/{volume}/files/content | AbilityWrite | handleDeleteVolumeFile |
| POST | /api/v1/apps/{name}/volumes/{volume}/files/restore | AbilityWrite | handleRestoreVolumeFile |

## App Storage / Database Attach

Endpoints for:
- [Object storage](/object-storage) bucket attachment and configuration
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
| GET | /api/v1/audit-log/export | AbilityRoot | handleExportAuditLog |
| GET | /api/v1/audit-log/verify | AbilityRoot | handleVerifyAuditLog |

`GET /api/v1/audit-log` accepts optional query params: `limit`, `before` (RFC3339 cursor), `path`, `method`, `client_kind` (exact match), `q` (case-insensitive substring across actor name, ability, method, path and remote address), `status=failed` (only `status_code >= 400`), and `format=csv`. All filters combine with AND and the CSV export honors them.

## AI Models / GPUs

AI model resources on GPU nodes and the GPU node snapshots they schedule against. Details: [AI models](/ai-models).

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| GET | /api/v1/models | AbilityRead | handleListModels |
| POST | /api/v1/models | AbilityWriteSensitive | handleCreateModel |
| GET | /api/v1/models/{name} | AbilityRead | handleGetModel |
| DELETE | /api/v1/models/{name} | AbilityWrite | handleDeleteModel |
| POST | /api/v1/models/{name}/restart | AbilityWrite | handleRestartModel |
| POST | /api/v1/models/{name}/api-key | AbilityWriteSensitive | handleRotateModelAPIKey |
| PUT | /api/v1/models/{name}/hf-token | AbilityWriteSensitive | handleSetModelHFToken |
| GET | /api/v1/models/{name}/logs | AbilityRead | handleQueryModelLogs |
| GET | /api/v1/models/{name}/logs/stream | AbilityRead | handleLiveModelLogStream |
| GET | /api/v1/gpus | AbilityRead | handleListGPUNodes |
| GET | /api/v1/models/{name}/keys | AbilityRead | handleListModelKeys |
| POST | /api/v1/models/{name}/keys | AbilityWriteSensitive | handleCreateModelKey |
| DELETE | /api/v1/models/{name}/keys/{id} | AbilityWrite | handleRevokeModelKey |
| POST | /api/v1/models/{name}/keys/{id}/rotate | AbilityWriteSensitive | handleRotateModelKey |
| GET | /api/v1/models/{name}/usage | AbilityRead | handleModelUsage |
| POST | /api/v1/models/preflight | AbilityRead | handleModelPreflight |
| GET | /api/v1/models/{name}/engine-metrics | AbilityRead | handleModelEngineMetrics |
| GET | /api/v1/models/{name}/fit | AbilityRead | handleModelFit |
| POST | /api/v1/models/fit | AbilityRead | handleModelFitCheck |
| PUT | /api/v1/models/{name}/residency | AbilityWrite | handleSetModelResidency |
| POST | /api/v1/models/{name}/wake | AbilityWrite | handleWakeModel |
| POST | /api/v1/models/{name}/sleep | AbilityWrite | handleSleepModel |
| PUT | /api/v1/models/{name}/swap-group | AbilityWrite | handleSetModelSwapGroup |
| POST | /api/v1/models/{name}/benchmark | AbilityWrite | handleModelBenchmark |
| POST | /api/v1/models/{name}/endpoint/test | AbilityWrite | handleTestEndpoint |
| PUT | /api/v1/models/{name}/endpoint/routing | AbilityWrite | handleSetEndpointRouting |
| PUT | /api/v1/models/{name}/endpoint/credential | AbilityWriteSensitive | handleSetEndpointCredential |

## Other

Routes that do not fit an existing group.

| Method | Path | Ability | Handler |
| --- | --- | --- | --- |
| POST | /api/v1/build/detect | AbilityDeploy | handleDetectFramework |
| POST | /api/v1/pipelines/validate | AbilityRead | handleValidatePipeline |
| GET | /api/v1/pipelines/schema | AbilityRead | handlePipelineSchema |
| POST | /api/v1/tags | AbilityWrite | handleCreateTag |
| GET | /api/v1/tags | AbilityRead | handleListTags |
| DELETE | /api/v1/tags/{id} | AbilityWrite | handleDeleteTag |
| GET | /api/v1/tags/{id}/apps | AbilityRead | handleListAppsByTag |
| GET | /api/v1/integrations | AbilityRead | handleListIntegrationCatalog |
| GET | /api/v1/deploy-approvals | AbilityRead | handleListDeployApprovals |
| GET | /api/v1/deploy-approvals/{id} | AbilityRead | handleGetDeployApproval |
| POST | /api/v1/deploy-approvals/{id}/approve | AbilityDeploy | handleApproveDeployApproval |
| POST | /api/v1/deploy-approvals/{id}/reject | AbilityDeploy | handleRejectDeployApproval |
| GET | /api/v1/mesh | AbilityRoot | handleGetMeshStatus |
| GET | /api/v1/gitea-app/connect | AbilityRoot | handleStartGitConnect |
| GET | /api/v1/gitea-app/callback | AbilityRoot | handleGitConnectCallback |
| POST | /api/v1/ai/sessions | AbilityRoot | handleCreateAIChatSession |
| GET | /api/v1/ai/sessions | AbilityRoot | handleListAIChatSessions |
| GET | /api/v1/ai/sessions/{id} | AbilityRoot | handleGetAIChatSession |
| DELETE | /api/v1/ai/sessions/{id} | AbilityRoot | handleDeleteAIChatSession |
| POST | /api/v1/ai/sessions/{id}/messages | AbilityRoot | handleCreateAIChatMessage |
| POST | /api/v1/ai/sessions/{id}/confirmations/{confirmation_id} | AbilityRoot | handleResolveAIChatConfirmation |
| GET | /api/v1/storage/providers | AbilityRead | handleListStorageProviders |
| GET | /api/v1/storage/destinations | AbilityRead | handleListStorageDestinations |
| POST | /api/v1/storage/destinations | AbilityWriteSensitive | handleCreateStorageDestination |
| GET | /api/v1/storage/destinations/{id} | AbilityRead | handleGetStorageDestination |
| PUT | /api/v1/storage/destinations/{id} | AbilityWriteSensitive | handleUpdateStorageDestination |
| DELETE | /api/v1/storage/destinations/{id} | AbilityWriteSensitive | handleDeleteStorageDestination |
| POST | /api/v1/storage/destinations/{id}/test | AbilityWriteSensitive | handleTestStorageDestination |
| GET | /api/v1/log-archive/policies | AbilityRead | handleListLogArchivePolicies |
| PUT | /api/v1/log-archive/policy | AbilityWrite | handleSetLogArchivePolicy |
| DELETE | /api/v1/log-archive/policy | AbilityWrite | handleDeleteLogArchivePolicy |
| POST | /api/v1/log-archive/dump | AbilityWrite | handleLogArchiveDump |
| GET | /api/v1/log-archive/runs | AbilityRead | handleListLogArchiveRuns |
| GET | /api/v1/log-archive/objects | AbilityRead | handleListLogArchiveObjects |
| GET | /api/v1/log-archive/objects/download | AbilityRead | handleDownloadLogArchiveObject |
| GET | /api/v1/build-cache | AbilityRead | handleListBuildCache |
| PUT | /api/v1/build-cache | AbilityWrite | handleSetBuildCache |
| DELETE | /api/v1/build-cache | AbilityWrite | handleDeleteBuildCache |
| GET | /api/v1/build-cache/stats | AbilityRead | handleBuildCacheStats |
| POST | /api/v1/build-cache/clear | AbilityWrite | handleClearBuildCache |
| GET | /api/v1/pipeline-runs | AbilityRead | handleListAllPipelineRuns |
| GET | /api/v1/pipelines/summary | AbilityRead | handleGetPipelineSummary |
| GET | /api/v1/loadbalancers | AbilityRead | handleListLoadBalancers |
| GET | /api/v1/alert-silences | AbilityRead | handleListAlertSilences |
| POST | /api/v1/alert-silences | AbilityWrite | handleCreateAlertSilence |
| DELETE | /api/v1/alert-silences/{id} | AbilityWrite | handleExpireAlertSilence |
| GET | /api/v1/alert-maintenance-windows | AbilityRead | handleListMaintenanceWindows |
| POST | /api/v1/alert-maintenance-windows | AbilityWrite | handleCreateMaintenanceWindow |
| PUT | /api/v1/alert-maintenance-windows/{id} | AbilityWrite | handleUpdateMaintenanceWindow |
| DELETE | /api/v1/alert-maintenance-windows/{id} | AbilityWrite | handleDeleteMaintenanceWindow |
| GET | /api/v1/alert-history | AbilityRead | handleListAlertHistory |
| GET | /public/status | Public | handlePublicStatusHTML |
| GET | /public/status.json | Public | handlePublicStatusJSON |
| GET | /public/status.rss | Public | handlePublicStatusRSS |
| GET | /api/v1/status-page | AbilityRead | handleGetStatusPage |
| PUT | /api/v1/status-page | AbilityWrite | handlePutStatusPage |
| GET | /api/v1/status-page/preview | AbilityRead | handleStatusPagePreview |
| GET | /api/v1/status-page/components | AbilityRead | handleListStatusComponents |
| POST | /api/v1/status-page/components | AbilityWrite | handleCreateStatusComponent |
| PUT | /api/v1/status-page/components/{id} | AbilityWrite | handleUpdateStatusComponent |
| DELETE | /api/v1/status-page/components/{id} | AbilityWrite | handleDeleteStatusComponent |
| GET | /api/v1/status-page/incidents | AbilityRead | handleListStatusIncidents |
| POST | /api/v1/status-page/incidents | AbilityWrite | handleCreateStatusIncident |
| POST | /api/v1/status-page/incidents/{id}/updates | AbilityWrite | handlePostStatusIncidentUpdate |
| DELETE | /api/v1/status-page/incidents/{id} | AbilityWrite | handleDeleteStatusIncident |
| GET | /api/v1/apps-summary | AbilityRead | handleAppsSummary |
| POST | /api/v1/preflight | AbilityWrite | handlePreflightNew |
| POST | /api/v1/imports/plan | AbilityWrite | handleImportPlan |
| POST | /api/v1/apply/plan | AbilityRead | handleIaCPlan |
| POST | /api/v1/apply | AbilityWrite | handleIaCApply |
| GET | /api/v1/export | AbilityRead | handleIaCExport |
| GET | /api/v1/apply/schema | AbilityRead | handleIaCSchema |
| POST | /api/v1/imports/platform/discover | AbilityWriteSensitive | handleDiscoverPlatformImport |
| POST | /api/v1/imports/platform/apply | AbilityWriteSensitive | handleApplyPlatformImport |
| GET | /api/v1/deployments | AbilityRead | handleListDeployments |
| GET | /api/v1/deployments/summary | AbilityRead | handleDeploymentsSummary |
| GET | /api/v1/deployments/stream | AbilityRead | handleDeploymentsStream |
| GET | /api/v1/model-cache | AbilityRead | handleListModelCache |
| POST | /api/v1/model-cache/prune | AbilityRoot | handlePruneModelCache |
| POST | /api/v1/pipelines/filters | AbilityRead | handlePipelineFilters |
| GET | /api/v1/apps-metrics | AbilityRead | handleBatchAppMetrics |
| GET | /api/v1/node-providers | AbilityRead | handleListNodeProviders |
| POST | /api/v1/node-providers | AbilityWriteSensitive | handleSetNodeProviderCredential |
| GET | /api/v1/node-providers/{provider}/regions | AbilityRead | handleListNodeProviderRegions |
| GET | /api/v1/node-providers/{provider}/sizes | AbilityRead | handleListNodeProviderSizes |
| GET | /api/v1/node-provisions | AbilityRoot | handleListNodeProvisions |
| GET | /api/v1/node-provisions/{id} | AbilityRoot | handleGetNodeProvision |
| GET | /.well-known/jwks.json | Public | handleOIDCJWKS |
| GET | /api/v1/pipelines/oidc | AbilityRead | handleGetPipelineOIDCInfo |
| POST | /api/v1/pipelines/oidc/rotate-key | AbilityRoot | handleRotatePipelineOIDCKey |
| GET | /api/v1/network/topology | AbilityRead | handleGetNetworkTopology |
| GET | /api/v1/ssh-node-provisions | AbilityRoot | handleListSSHNodeProvisions |
| GET | /api/v1/ssh-node-provisions/{id} | AbilityRoot | handleGetSSHNodeProvision |
| GET | /api/v1/network-shares | AbilityRead | handleListNetworkShares |
| POST | /api/v1/network-shares | AbilityWriteSensitive | handleCreateNetworkShare |
| GET | /api/v1/network-shares/{id} | AbilityRead | handleGetNetworkShare |
| PUT | /api/v1/network-shares/{id} | AbilityWriteSensitive | handleUpdateNetworkShare |
| DELETE | /api/v1/network-shares/{id} | AbilityWriteSensitive | handleDeleteNetworkShare |
| POST | /api/v1/network-shares/{id}/test | AbilityRead | handleTestNetworkShare |
| GET | /api/v1/firewall-rules | AbilityRead | handleListFirewallRules |
| POST | /api/v1/firewall-rules | AbilityWriteSensitive | handleCreateFirewallRule |
| DELETE | /api/v1/firewall-rules/{id} | AbilityWriteSensitive | handleDeleteFirewallRule |
| GET | /api/v1/openapi.json | AbilityRead | handleOpenAPISpec |
| GET | /api/v1/changelog | AbilityRead | handleGetChangelog |
| GET | /api/v1/templates/custom | AbilityRead | handleListCustomTemplates |
| DELETE | /api/v1/templates/custom/{id} | AbilityWrite | handleDeleteCustomTemplate |
| GET | /api/v1/network/proxy | AbilityRead | handleGetNetworkProxy |
| GET | /api/v1/firewall/host | AbilityRead | handleHostFirewallStatus |
| POST | /api/v1/firewall/host/enable | AbilityRoot | handleEnableHostFirewall |
| POST | /api/v1/firewall/host/disable | AbilityRoot | handleDisableHostFirewall |
| POST | /api/v1/hooks/image-update/{name}/{token} | Public | handleImageUpdateWebhook |
| GET | /api/v1/hooks/wake | Public | handleWakeHook |
| GET | /api/v1/imports/platform/databases | AbilityRead | handleListDatabaseDataCopies |
| GET | /api/v1/imports/platform/databases/{name} | AbilityRead | handleGetDatabaseDataCopy |
| POST | /api/v1/imports/platform/databases/{name}/copy | AbilityWriteSensitive | handleCopyDatabaseData |
| GET | /api/v1/migration/volumes | AbilityRead | handleVolumeGuide |
| GET | /api/v1/migration/cutover | AbilityRead | handleCutoverReport |
| GET | /api/v1/migration/cutover/verify | AbilityWriteSensitive | handleCutoverVerify |
| GET | /api/v1/ingress/connectivity | AbilityRead | handleIngressConnectivity |
| GET | /api/v1/firewall/exposure | AbilityRead | handleExposureReport |
| POST | /api/v1/firewall/exposure/preview | AbilityRoot | handleExposurePreview |
| PUT | /api/v1/firewall/exposure/restrictions/{protocol}/{port} | AbilityRoot | handleExposureRestrict |
| DELETE | /api/v1/firewall/exposure/restrictions/{protocol}/{port} | AbilityRoot | handleExposureUnrestrict |
| GET | /api/v1/attention/feed | AbilityRead | handleAttentionFeed |
| POST | /api/v1/attention/dismiss | Session | handleAttentionDismiss |
| GET | /api/v1/external-databases | AbilityRead | handleListExternalDatabases |
| POST | /api/v1/external-databases | AbilityRoot | handleCreateExternalDatabase |
| POST | /api/v1/external-databases/test | AbilityRoot | handleTestExternalDatabase |
| GET | /api/v1/external-databases/candidates | AbilityRoot | handleListExternalDatabaseCandidates |
| POST | /api/v1/external-databases/adopt | AbilityRoot | handleAdoptExternalDatabase |
| GET | /api/v1/external-databases/{name} | AbilityRead | handleGetExternalDatabase |
| PUT | /api/v1/external-databases/{name} | AbilityRoot | handleUpdateExternalDatabase |
| DELETE | /api/v1/external-databases/{name} | AbilityWrite | handleDeleteExternalDatabase |
| POST | /api/v1/external-databases/{name}/probe | AbilityWrite | handleProbeExternalDatabase |
| GET | /api/v1/external-databases/{name}/password | AbilityRoot | handleRevealExternalDatabasePassword |
| GET | /api/v1/usage/summary | AbilityRead | handleUsageSummary |
| GET | /api/v1/migration/hub/local-sources | AbilityRead | handleHubLocalSources |
| POST | /api/v1/migration/hub/sessions | AbilityWriteSensitive | handleCreateHubSession |
| GET | /api/v1/migration/hub/sessions | AbilityRead | handleListHubSessions |
| GET | /api/v1/migration/hub/sessions/{id} | AbilityRead | handleGetHubSession |
| DELETE | /api/v1/migration/hub/sessions/{id} | AbilityWriteSensitive | handleDeleteHubSession |
| PUT | /api/v1/migration/hub/sessions/{id}/selection | AbilityWriteSensitive | handlePutHubSelection |
| PUT | /api/v1/migration/hub/sessions/{id}/step | AbilityWriteSensitive | handlePutHubStep |
| POST | /api/v1/migration/hub/sessions/{id}/apply | AbilityWriteSensitive | handleApplyHubSession |
| GET | /api/v1/migration/hub/sessions/{id}/receipt | AbilityRead | handleHubReceipt |
| GET | /api/v1/migration/hub/sessions/{id}/items/{db}/connection | AbilityRead | handleHubConnection |
| POST | /api/v1/migration/hub/sessions/{id}/items/{db}/reveal | AbilityReadSensitive | handleHubReveal |
| POST | /api/v1/migration/apps/plan | AbilityWriteSensitive | handleAppImportPlan |
| POST | /api/v1/migration/apps/sessions | AbilityWriteSensitive | handleCreateAppImportSession |
| GET | /api/v1/migration/apps/sessions | AbilityRead | handleListAppImportSessions |
| GET | /api/v1/migration/apps/sessions/{id} | AbilityRead | handleGetAppImportSession |
| DELETE | /api/v1/migration/apps/sessions/{id} | AbilityWriteSensitive | handleDeleteAppImportSession |
| POST | /api/v1/migration/apps/sessions/{id}/connect | AbilityWriteSensitive | handleConnectAppImportSession |
| PUT | /api/v1/migration/apps/sessions/{id}/plan | AbilityWriteSensitive | handlePutAppImportPlan |
| POST | /api/v1/migration/apps/sessions/{id}/stage | AbilityWriteSensitive | handleStageAppImport |
| POST | /api/v1/migration/apps/sessions/{id}/verify | AbilityWriteSensitive | handleVerifyAppImport |
| POST | /api/v1/migration/apps/sessions/{id}/rollback | AbilityWriteSensitive | handleRollbackAppImport |
| GET | /api/v1/migration/apps/sessions/{id}/images | AbilityRead | handleAppImportImages |
| GET | /api/v1/migration/apps/sessions/{id}/images/status | AbilityRead | handleAppImportImagesStatus |
| POST | /api/v1/migration/apps/sessions/{id}/images/transfer | AbilityWriteSensitive | handleTransferAppImportImages |
| POST | /api/v1/migration/apps/sessions/{id}/images/cancel | AbilityWriteSensitive | handleCancelAppImportImages |
| GET | /api/v1/migration/apps/sessions/{id}/volumes | AbilityRead | handleAppImportVolumes |
| PUT | /api/v1/migration/apps/sessions/{id}/items/{item}/volumes/{vol} | AbilityWriteSensitive | handleSetAppImportVolume |
| GET | /api/v1/migration/apps/sessions/{id}/cutover | AbilityRead | handleAppImportCutover |
| GET | /api/v1/migration/apps/sessions/{id}/cutover/verify | AbilityWriteSensitive | handleAppImportCutoverVerify |
| POST | /api/v1/migration/apps/sessions/{id}/items/{item}/route | AbilityWriteSensitive | handleRouteAppImport |
| GET | /api/v1/migration/apps/sessions/{id}/receipt | AbilityRead | handleAppImportReceipt |
| GET | /api/v1/dns/zones | AbilityRead | handleListDNSZones |
| POST | /api/v1/dns/zones | AbilityRoot | handleCreateDNSZone |
| GET | /api/v1/dns/zones/{zone} | AbilityRead | handleGetDNSZone |
| DELETE | /api/v1/dns/zones/{zone} | AbilityRoot | handleDeleteDNSZone |
| GET | /api/v1/dns/zones/{zone}/nameservers | AbilityRead | handleDNSZoneNameServers |
| GET | /api/v1/dns/zones/{zone}/delegation | AbilityRead | handleDNSZoneDelegation |
| GET | /api/v1/dns/zones/{zone}/discover | AbilityRead | handleDiscoverDNSZoneRecords |
| GET | /api/v1/dns/zones/{zone}/records | AbilityRead | handleListDNSZoneRecords |
| POST | /api/v1/dns/zones/{zone}/records | AbilityRoot | handleCreateDNSZoneRecord |
| PUT | /api/v1/dns/zones/{zone}/records | AbilityRoot | handleUpdateDNSZoneRecord |
| DELETE | /api/v1/dns/zones/{zone}/records | AbilityRoot | handleDeleteDNSZoneRecord |
| POST | /api/v1/dns/zones/{zone}/records/import | AbilityRoot | handleImportDNSZoneRecords |
| GET | /api/v1/dns/zones/{zone}/records/export | AbilityRead | handleExportDNSZoneRecords |
| POST | /api/v1/dns/zones/{zone}/templates/{id} | AbilityRoot | handleApplyDNSTemplate |
| GET | /api/v1/dns/templates | AbilityRead | handleListDNSTemplates |
| GET | /api/v1/dns/check | AbilityRead | handleDNSCheck |
| GET | /api/v1/dns/health-checks | AbilityRead | handleListDNSHealthChecks |
| POST | /api/v1/dns/health-checks | AbilityRoot | handleCreateDNSHealthCheck |
| DELETE | /api/v1/dns/health-checks/{id} | AbilityRoot | handleDeleteDNSHealthCheck |
| GET | /api/v1/dns/zone | AbilityRead | handleDNSZone |
| GET | /api/v1/traffic/summary | AbilityRead | handleTrafficSummary |
| GET | /api/v1/migration/apps/sessions/{id}/items/{item}/cutover/plan | AbilityRead | handleAppImportCutoverPlan |
| GET | /api/v1/migration/apps/sessions/{id}/items/{item}/cutover/runs | AbilityRead | handleListAppImportCutoverRuns |
| POST | /api/v1/migration/apps/sessions/{id}/items/{item}/cutover/runs | AbilityWriteSensitive | handleStartAppImportCutover |
| GET | /api/v1/migration/apps/sessions/{id}/items/{item}/cutover/runs/{run} | AbilityRead | handleGetAppImportCutoverRun |
| POST | /api/v1/migration/apps/sessions/{id}/items/{item}/cutover/runs/{run}/rollback | AbilityWriteSensitive | handleRollbackAppImportCutover |
| POST | /api/v1/migration/apps/sessions/{id}/items/{item}/cutover/runs/{run}/confirm-dns | AbilityWriteSensitive | handleConfirmAppImportCutoverDNS |
| GET | /api/v1/compose | AbilityRead | handleListComposeStacks |
| POST | /api/v1/compose/preview | AbilityDeploy | handlePreviewCompose |
| GET | /api/v1/node-moves | AbilityRoot | handleListNodeMoves |
| GET | /api/v1/node-moves/{id} | AbilityRoot | handleGetNodeMove |
| GET | /api/v1/node-drains/{id} | AbilityRoot | handleGetNodeDrain |
| GET | /api/v1/mesh/connectivity | AbilityRoot | handleMeshConnectivity |
| GET | /api/v1/teams | AbilityRead | handleListTeams |
| POST | /api/v1/teams | AbilityRoot | handleCreateTeam |
| GET | /api/v1/teams/roles | AbilityRead | handleListTeamRoles |
| GET | /api/v1/teams/{id} | AbilityRead | handleGetTeam |
| PATCH | /api/v1/teams/{id} | AbilityWrite | handleRenameTeam |
| DELETE | /api/v1/teams/{id} | AbilityWrite | handleDeleteTeam |
| GET | /api/v1/teams/{id}/members | AbilityRead | handleListTeamMembers |
| PUT | /api/v1/teams/{id}/members/{user} | AbilityWrite | handleSetTeamMemberRole |
| DELETE | /api/v1/teams/{id}/members/{user} | AbilityWrite | handleRemoveTeamMember |
| DELETE | /api/v1/teams/{id}/members/me | AbilityRead | handleLeaveTeam |
| POST | /api/v1/teams/{id}/transfer | AbilityWrite | handleTransferTeam |
| GET | /api/v1/teams/{id}/events | AbilityRead | handleListTeamEvents |
| GET | /api/v1/teams/{id}/invites | AbilityRead | handleListTeamInvites |
| POST | /api/v1/teams/{id}/invites | AbilityWrite | handleCreateTeamInvite |
| DELETE | /api/v1/teams/{id}/invites/{invite} | AbilityWrite | handleRevokeTeamInvite |
| GET | /api/v1/team-invites/{token} | Session | handlePreviewTeamInvite |
| POST | /api/v1/team-invites/accept | Public | handleAcceptTeamInvite |
| GET | /metrics | AbilityRead | handleScrapeMetrics |
| GET | /api/v1/export/destinations | AbilityRoot | handleListExportDestinations |
| POST | /api/v1/export/destinations | AbilityRoot | handleCreateExportDestination |
| GET | /api/v1/export/destinations/{id} | AbilityRoot | handleGetExportDestination |
| PATCH | /api/v1/export/destinations/{id} | AbilityRoot | handleUpdateExportDestination |
| DELETE | /api/v1/export/destinations/{id} | AbilityRoot | handleDeleteExportDestination |
| POST | /api/v1/export/destinations/{id}/test | AbilityRoot | handleTestExportDestination |
| GET | /api/v1/mesh/settings | AbilityRead | handleGetMeshSettings |
| PUT | /api/v1/mesh/settings | AbilityRoot | handleUpdateMeshSettings |
| GET | /api/v1/resource-advisor/findings | AbilityRead | handleResourceAdvisorFindings |
| GET | /api/v1/resource-advisor/summary | AbilityRead | handleResourceAdvisorSummary |
| POST | /api/v1/resource-advisor/findings/{id}/apply | AbilityWrite | handleResourceAdvisorApply |
| POST | /api/v1/resource-advisor/findings/{id}/dismiss | AbilityWrite | handleResourceAdvisorState |
| POST | /api/v1/resource-advisor/findings/{id}/snooze | AbilityWrite | handleResourceAdvisorState |
| POST | /api/v1/resource-advisor/findings/{id}/restore | AbilityWrite | handleResourceAdvisorState |
| POST | /api/v1/imports/analyze | AbilityWrite | handleAnalyzeImport |
| GET | /api/v1/storage/orphans | AbilityRead | handleListStorageOrphans |
| DELETE | /api/v1/storage/orphans/{id} | AbilityWriteSensitive | handleReclaimStorageOrphan |
| GET | /api/v1/setup/guide | AbilityRead | handleGetSetupGuide |
| PUT | /api/v1/setup/guide/steps/{id} | AbilityRoot | handleSetSetupGuideStep |
| PUT | /api/v1/setup/guide/visibility | AbilityRoot | handleSetSetupGuideVisibility |
| POST | /api/v1/setup/guide/reset | AbilityRoot | handleResetSetupGuide |
| GET | /api/v1/logs/queries | AbilityRead | handleListSavedLogQueries |
| POST | /api/v1/logs/queries | AbilityWrite | handleCreateSavedLogQuery |
| PUT | /api/v1/logs/queries/{id} | AbilityWrite | handleUpdateSavedLogQuery |
| DELETE | /api/v1/logs/queries/{id} | AbilityWrite | handleDeleteSavedLogQuery |
| GET | /api/v1/usage/report | AbilityRead | handleUsageReport |
| GET | /api/v1/usage/settings | AbilityRoot | handleGetUsageSettings |
| PUT | /api/v1/usage/settings | AbilityRoot | handlePutUsageSettings |
| GET | /api/v1/usage/budgets | AbilityRead | handleListUsageBudgets |
| PUT | /api/v1/usage/budgets | AbilityWrite | handlePutUsageBudget |
| DELETE | /api/v1/usage/budgets/{id} | AbilityWrite | handleDeleteUsageBudget |
| POST | /api/v1/mobile/pairing-codes | AbilityRoot | handleCreateMobilePairing |
| POST | /api/v1/mobile/pair | Public | handleMobilePair |
| GET | /api/v1/mobile/devices | AbilityRead | handleListMobileDevices |
| DELETE | /api/v1/mobile/devices/{id} | AbilityRead | handleRevokeMobileDevice |
| GET | /api/v1/mobile/overview | AbilityRead | handleMobileOverview |
| GET | /api/v1/gate/check | Public | handleGateCheck |
| GET | /gate/login | Public | handleGateLogin |
| GET | /api/v1/demo | Public | handleDemoInfo |
| POST | /api/v1/demo/session | Public | handleDemoSession |
| POST | /api/v1/demo/cta | Public | handleDemoCTA |
| GET | /api/v1/deployments/timeline | AbilityRead | handleDeploymentsTimeline |
| GET | /api/v1/event-webhooks | AbilityRoot | handleListEventWebhooks |
| POST | /api/v1/event-webhooks | AbilityRoot | handleCreateEventWebhook |
| GET | /api/v1/event-webhook-types | AbilityRoot | handleListEventWebhookTypes |
| GET | /api/v1/event-webhooks/{id} | AbilityRoot | handleGetEventWebhook |
| PATCH | /api/v1/event-webhooks/{id} | AbilityRoot | handleUpdateEventWebhook |
| DELETE | /api/v1/event-webhooks/{id} | AbilityRoot | handleDeleteEventWebhook |
| POST | /api/v1/event-webhooks/{id}/test | AbilityRoot | handleTestEventWebhook |
| POST | /api/v1/event-webhooks/{id}/rotate-secret | AbilityRoot | handleRotateEventWebhookSecret |
| GET | /api/v1/event-webhooks/{id}/deliveries | AbilityRoot | handleListEventWebhookDeliveries |
| POST | /api/v1/event-webhook-deliveries/{id}/replay | AbilityRoot | handleReplayEventWebhookDelivery |
| GET | /api/v1/secret-rotation | AbilityRoot | handleListAllSecretRotation |
| GET | /api/v1/teams/{id}/quota | AbilityRead | handleGetQuota |
| PUT | /api/v1/teams/{id}/quota | AbilityWrite | handleSetQuota |
| DELETE | /api/v1/teams/{id}/quota | AbilityWrite | handleDeleteQuota |
| GET | /api/v1/scheduled-maintenance | AbilityRead | handleListScheduledMaintenance |
| POST | /api/v1/scheduled-maintenance | AbilityWriteSensitive | handleCreateScheduledMaintenance |
| DELETE | /api/v1/scheduled-maintenance/{id} | AbilityWriteSensitive | handleDeleteScheduledMaintenance |
| GET | /api/v1/compliance/access-reviews | AbilityRoot | handleListAccessReviews |
| POST | /api/v1/compliance/access-reviews | AbilityRoot | handleCreateAccessReview |
| GET | /api/v1/compliance/access-reviews/{id} | AbilityRoot | handleGetAccessReview |
| PUT | /api/v1/compliance/access-reviews/{id}/items/{item} | AbilityRoot | handleDecideAccessReviewItem |
| POST | /api/v1/compliance/access-reviews/{id}/complete | AbilityRoot | handleCompleteAccessReview |
| GET | /api/v1/compliance/evidence-packs | AbilityRoot | handleListEvidencePacks |
| POST | /api/v1/compliance/evidence-packs | AbilityRoot | handleCreateEvidencePack |
| GET | /api/v1/compliance/evidence-packs/{id} | AbilityRoot | handleGetEvidencePack |
| GET | /api/v1/compliance/evidence-packs/{id}/download | AbilityRoot | handleDownloadEvidencePack |
| POST | /api/v1/compliance/evidence-packs/{id}/verify | AbilityRoot | handleVerifyEvidencePack |
| GET | /api/v1/sso/saml/connections | AbilityRoot | handleListSAMLConnections |
| POST | /api/v1/sso/saml/connections | AbilityRoot | handleCreateSAMLConnection |
| GET | /api/v1/sso/saml/connections/{id} | AbilityRoot | handleGetSAMLConnection |
| PUT | /api/v1/sso/saml/connections/{id} | AbilityRoot | handleUpdateSAMLConnection |
| DELETE | /api/v1/sso/saml/connections/{id} | AbilityRoot | handleDeleteSAMLConnection |
| POST | /api/v1/sso/saml/connections/{id}/metadata | AbilityRoot | handleImportSAMLMetadata |
| GET | /api/v1/sso/group-mappings | AbilityRoot | handleListSSOGroupMappings |
| PUT | /api/v1/sso/group-mappings | AbilityRoot | handlePutSSOGroupMappings |
| GET | /api/v1/sso/settings | AbilityRoot | handleGetSSOSettings |
| PUT | /api/v1/sso/settings | AbilityRoot | handlePutSSOSettings |
| GET | /api/v1/sso/scim/tokens | AbilityRoot | handleListSCIMTokens |
| POST | /api/v1/sso/scim/tokens | AbilityRoot | handleCreateSCIMToken |
| POST | /api/v1/sso/scim/tokens/{id}/rotate | AbilityRoot | handleRotateSCIMToken |
| DELETE | /api/v1/sso/scim/tokens/{id} | AbilityRoot | handleRevokeSCIMToken |
| GET | /api/v1/scim/v2/ServiceProviderConfig | Public | handleSCIM |
| GET | /api/v1/scim/v2/ResourceTypes | Public | handleSCIM |
| GET | /api/v1/scim/v2/Users | Public | handleSCIM |
| POST | /api/v1/scim/v2/Users | Public | handleSCIM |
| GET | /api/v1/scim/v2/Users/{id} | Public | handleSCIM |
| PUT | /api/v1/scim/v2/Users/{id} | Public | handleSCIM |
| PATCH | /api/v1/scim/v2/Users/{id} | Public | handleSCIM |
| DELETE | /api/v1/scim/v2/Users/{id} | Public | handleSCIM |
| GET | /api/v1/scim/v2/Groups | Public | handleSCIM |
| POST | /api/v1/scim/v2/Groups | Public | handleSCIM |
| GET | /api/v1/scim/v2/Groups/{id} | Public | handleSCIM |
| PUT | /api/v1/scim/v2/Groups/{id} | Public | handleSCIM |
| PATCH | /api/v1/scim/v2/Groups/{id} | Public | handleSCIM |
| DELETE | /api/v1/scim/v2/Groups/{id} | Public | handleSCIM |
| POST | /api/v1/node-providers/{provider}/test | AbilityWriteSensitive | handleTestNodeProvider |
| DELETE | /api/v1/node-providers/{provider} | AbilityWriteSensitive | handleDisconnectNodeProvider |
| GET | /api/v1/registry-vendors | AbilityRead | handleListRegistryVendors |
| GET | /api/v1/package-registries | AbilityRead | handleListPackageRegistries |
| POST | /api/v1/package-registries | AbilityWriteSensitive | handleCreatePackageRegistry |
| DELETE | /api/v1/package-registries/{id} | AbilityWriteSensitive | handleDeletePackageRegistry |
| POST | /api/v1/package-registries/{id}/test | AbilityWriteSensitive | handleTestPackageRegistry |
| GET | /api/v1/database-versions | AbilityRead | handleListDatabaseVersions |
| POST | /api/v1/database-versions/refresh | AbilityWriteSensitive | handleRefreshDatabaseVersions |
| GET | /api/v1/database-catalog | AbilityRead | handleGetDatabaseCatalogStatus |
| POST | /api/v1/database-catalog/refresh | AbilityWriteSensitive | handleRefreshDatabaseCatalog |
| GET | /api/v1/scim/v2/ResourceTypes/{id} | Public | handleSCIM |
| GET | /api/v1/scim/v2/Schemas | Public | handleSCIM |
| GET | /api/v1/scim/v2/Schemas/{id} | Public | handleSCIM |
| GET | /api/v1/sleep | AbilityRead | handleSleepFleet |
| GET | /api/v1/deploy-policies | AbilityRead | handleListDeployPolicies |
| POST | /api/v1/deploy-policies/check | AbilityRead | handleCheckDeployPolicy |
| PUT | /api/v1/deploy-policies/{scope}/{id} | AbilityRoot | handleSetDeployPolicy |
| DELETE | /api/v1/deploy-policies/{scope}/{id} | AbilityRoot | handleDeleteDeployPolicy |
| POST | /api/v1/otlp/v1/traces | Public | handleOTLPTraces |
| POST | /api/v1/sentry/api/{project}/envelope/ | Public | handleSentryEnvelope |
| POST | /api/v1/sentry/api/{project}/store/ | Public | handleSentryStore |
| OPTIONS | /api/v1/sentry/api/{project}/envelope/ | Public | handleSentryPreflight |
| OPTIONS | /api/v1/sentry/api/{project}/store/ | Public | handleSentryPreflight |
| GET | /api/v1/logs/usage | AbilityRead | handleLogUsage |
| GET | /api/v1/sleep/overview | AbilityRead | handleSleepOverview |
| GET | /api/v1/fixes | AbilityRead | handleListFixes |
| POST | /api/v1/fixes/{id}/preview | AbilityRead | handlePreviewFix |
| POST | /api/v1/fixes/{id}/apply | AbilityRoot | handleApplyFix |
| GET | /api/v1/build-limits | AbilityRead | handleGetBuildLimits |
| PUT | /api/v1/build-limits | AbilityRoot | handleSetBuildLimits |
| DELETE | /api/v1/build-limits | AbilityRoot | handleResetBuildLimits |
| GET | /api/v1/maintenance/domains | AbilityRead | handleListFleetMaintenance |
| POST | /api/v1/maintenance/preview | AbilityRead | handlePreviewMaintenance |
| POST | /api/v1/git-sources/analyze | AbilityWrite | handleAnalyzeImport |
| POST | /api/v1/hooks/pipeline/{id}/{token} | Public | handlePipelineWebhook |
| GET | /api/v1/ai/usage | AbilityRead | handleAIUsage |
| GET | /api/v1/model-catalog | AbilityRead | handleModelCatalog |
| GET | /api/v1/plugins | AbilityRoot | handleListPlugins |
| GET | /api/v1/plugins/{name} | AbilityRoot | handleGetPlugin |
| POST | /api/v1/plugins | AbilityRoot | handleInstallPlugin |
| POST | /api/v1/plugins/{name}/approve | AbilityRoot | handleApprovePlugin |
| POST | /api/v1/plugins/{name}/enable | AbilityRoot | handleEnablePlugin |
| POST | /api/v1/plugins/{name}/disable | AbilityRoot | handleDisablePlugin |
| DELETE | /api/v1/plugins/{name} | AbilityRoot | handleRemovePlugin |
| GET | /api/v1/plugin-keys | AbilityRoot | handleListPluginKeys |
| POST | /api/v1/plugin-keys | AbilityRoot | handleAddPluginKey |
| DELETE | /api/v1/plugin-keys/{id} | AbilityRoot | handleRemovePluginKey |
| GET | /api/v1/cdn/overview | AbilityRead | handleCDNOverview |
| GET | /api/v1/firewall/edge/status | AbilityRead | handleFirewallEdgeStatus |
| GET | /api/v1/firewall/edge/series | AbilityRead | handleFirewallEdgeSeries |
| GET | /api/v1/firewall/edge/traffic | AbilityReadSensitive | handleFirewallEdgeTraffic |
| GET | /api/v1/firewall/edge/rules | AbilityRead | handleListFirewallEdgeRules |
| POST | /api/v1/firewall/edge/rules | AbilityDeploy | handleCreateFirewallEdgeRule |
| POST | /api/v1/firewall/edge/rules/order | AbilityDeploy | handleReorderFirewallEdgeRules |
| PUT | /api/v1/firewall/edge/rules/{id} | AbilityDeploy | handleUpdateFirewallEdgeRule |
| DELETE | /api/v1/firewall/edge/rules/{id} | AbilityDeploy | handleDeleteFirewallEdgeRule |
| POST | /api/v1/firewall/edge/block | AbilityDeploy | handleFirewallIPEntry |
| POST | /api/v1/firewall/edge/allow | AbilityDeploy | handleFirewallIPEntry |
| GET | /api/v1/firewall/edge/bans | AbilityReadSensitive | handleListFirewallEdgeBans |
| DELETE | /api/v1/firewall/edge/bans/{ip} | AbilityDeploy | handleLiftFirewallEdgeBan |
| GET | /api/v1/firewall/edge/settings | AbilityRead | handleGetFirewallEdgeSettings |
| PUT | /api/v1/firewall/edge/settings | AbilityDeploy | handleSetFirewallEdgeSettings |
| POST | /api/v1/firewall/edge/attack-mode | AbilityDeploy | handleStartFirewallEdgeAttack |
| DELETE | /api/v1/firewall/edge/attack-mode | AbilityDeploy | handleStopFirewallEdgeAttack |
| GET | /api/v1/firewall/edge/audit | AbilityRead | handleFirewallEdgeAudit |
| GET | /api/v1/images | AbilityRead | handleListImageInventory |
| GET | /api/v1/images/layers | AbilityRead | handleImageLayerBreakdown |
| POST | /api/v1/images/verify | AbilityWrite | handleImageVerify |
| POST | /api/v1/images/gc | AbilityRoot | handleImageGC |
| GET | /api/v1/functions | AbilityRead | handleListFunctions |
| GET | /api/v1/functions/runtimes | AbilityRead | handleFunctionRuntimes |
| GET | /api/v1/storage/buckets | AbilityRead | handleListStorageBuckets |
| POST | /api/v1/storage/buckets | AbilityWriteSensitive | handleCreateStorageBucket |
| GET | /api/v1/storage/buckets/{id} | AbilityRead | handleGetStorageBucket |
| PATCH | /api/v1/storage/buckets/{id} | AbilityWriteSensitive | handleUpdateStorageBucket |
| DELETE | /api/v1/storage/buckets/{id} | AbilityWriteSensitive | handleDeleteStorageBucket |
| GET | /api/v1/storage/buckets/{id}/usage | AbilityRead | handleBucketUsage |
| GET | /api/v1/storage/buckets/{id}/objects | AbilityReadSensitive | handleListBucketObjects |
| PUT | /api/v1/storage/buckets/{id}/objects | AbilityWrite | handleUploadBucketObject |
| DELETE | /api/v1/storage/buckets/{id}/objects | AbilityWrite | handleDeleteBucketObject |
| GET | /api/v1/storage/buckets/{id}/objects/download | AbilityReadSensitive | handleDownloadBucketObject |
| POST | /api/v1/storage/buckets/{id}/presign | AbilityReadSensitive | handlePresignBucketObject |
| POST | /api/v1/storage/buckets/{id}/lifecycle/run | AbilityWrite | handleSweepStorageBucket |
| GET | /api/v1/private-networks | AbilityRead | handleListPrivateNetworks |
| POST | /api/v1/private-networks | AbilityRoot | handleCreatePrivateNetwork |
| GET | /api/v1/private-networks/map | AbilityRead | handlePrivateNetworkMap |
| POST | /api/v1/private-networks/apply | AbilityRoot | handleApplyPrivateNetworks |
| POST | /api/v1/private-networks/rules | AbilityRoot | handleCreatePrivateNetworkRule |
| DELETE | /api/v1/private-networks/rules/{id} | AbilityRoot | handleDeletePrivateNetworkRule |
| PUT | /api/v1/private-networks/{id} | AbilityRoot | handleUpdatePrivateNetwork |
| DELETE | /api/v1/private-networks/{id} | AbilityRoot | handleDeletePrivateNetwork |
| GET | /api/v1/resources | AbilityRead | handleListResources |
| POST | /api/v1/resources/tags/bulk | AbilityWrite | handleBulkResourceTags |
| GET | /api/v1/resources/views | AbilityWrite | handleListResourceViews |
| POST | /api/v1/resources/views | AbilityWrite | handleSaveResourceView |
| DELETE | /api/v1/resources/views/{id} | AbilityWrite | handleDeleteResourceView |
| GET | /api/v1/functions/{name}/route | AbilityRead | handleListFunctionRoutes |
| PUT | /api/v1/functions/{name}/route | AbilityDeploy | handleSetFunctionRoute |
| DELETE | /api/v1/functions/{name}/route | AbilityDeploy | handleDeleteFunctionRoute |
| GET | /api/v1/dashboards | AbilityRead | handleListDashboards |
| GET | /api/v1/dashboards/templates | AbilityRead | handleDashboardTemplates |
| POST | /api/v1/dashboards/import | AbilityWrite | handleImportDashboard |
| POST | /api/v1/dashboards | AbilityWrite | handleCreateDashboard |
| GET | /api/v1/dashboards/{id} | AbilityRead | handleGetDashboard |
| GET | /api/v1/dashboards/{id}/export | AbilityRead | handleExportDashboard |
| PUT | /api/v1/dashboards/{id} | AbilityWrite | handleUpdateDashboard |
| DELETE | /api/v1/dashboards/{id} | AbilityWrite | handleDeleteDashboard |
| GET | /api/v1/builds | AbilityRead | handleListBuilds |
| GET | /api/v1/builds/cache | AbilityRead | handleBuildCache |
| POST | /api/v1/builds/cache/prune | AbilityWriteSensitive | handleBuildCachePrune |
| GET | /api/v1/builds/{id} | AbilityRead | handleGetBuild |
| POST | /api/v1/plugins/inspect | AbilityRoot | handleInspectPlugin |
| POST | /api/v1/plugins/{name}/upgrade | AbilityRoot | handleUpgradePlugin |
| POST | /api/v1/plugins/{name}/restart | AbilityRoot | handleRestartPlugin |
| GET | /api/v1/costs/explore | AbilityRead | handleCostExplore |
| GET | /api/v1/costs/budgets | AbilityRead | handleListCostBudgets |
| POST | /api/v1/costs/budgets | AbilityRoot | handleCreateCostBudget |
| PUT | /api/v1/costs/budgets/{id} | AbilityRoot | handleUpdateCostBudget |
| DELETE | /api/v1/costs/budgets/{id} | AbilityRoot | handleDeleteCostBudget |
| POST | /api/v1/migration/plan | AbilityWriteSensitive | handleMigrationPlan |
| POST | /api/v1/migration/import | AbilityWriteSensitive | handleMigrationImport |
| GET | /api/v1/migration/checklists | AbilityRead | handleListMigrationChecklists |
| GET | /api/v1/migration/checklists/{app} | AbilityRead | handleGetMigrationChecklist |
| POST | /api/v1/migration/checklists/{app}/steps/{step} | AbilityWriteSensitive | handleMigrationChecklistStep |
| GET | /api/v1/resilience | AbilityRead | handleChaosReport |
| POST | /api/v1/resilience/results | AbilityWrite | handleRecordChaosResults |
| GET | /api/v1/pipelines/templates | AbilityRead | handleListPipelineTemplates |
| GET | /api/v1/pipelines/templates/{id} | AbilityRead | handleGetPipelineTemplate |
| GET | /api/v1/project-templates | AbilityRead | handleListProjectTemplates |
| POST | /api/v1/project-templates/{id}/render | AbilityRead | handleRenderProjectTemplate |
| GET | /api/v1/archives | AbilityWriteSensitive | handleListArchives |
| POST | /api/v1/archives | AbilityWriteSensitive | handleCreateArchive |
| GET | /api/v1/archives/policy | AbilityRoot | handleGetArchivePolicy |
| PUT | /api/v1/archives/policy | AbilityRoot | handlePutArchivePolicy |
| GET | /api/v1/archives/{id} | AbilityRoot | handleGetArchive |
| GET | /api/v1/archives/{id}/rows | AbilityRoot | handleBrowseArchive |
| GET | /api/v1/archives/{id}/download | AbilityRoot | handleDownloadArchive |
| POST | /api/v1/archives/{id}/restore | AbilityRoot | handleRestoreArchive |
| POST | /api/v1/archives/{id}/verify | AbilityRoot | handleVerifyArchive |
| DELETE | /api/v1/archives/{id} | AbilityRoot | handleDeleteArchive |
| GET | /api/v1/storage/defaults | AbilityRead | handleListStorageDefaults |
| PUT | /api/v1/storage/defaults/{class} | AbilityWriteSensitive | handleSetStorageDefault |
| DELETE | /api/v1/storage/defaults/{class} | AbilityWriteSensitive | handleClearStorageDefault |
| GET | /api/v1/frameworks | AbilityRead | handleListFrameworks |
| GET | /api/v1/frameworks/{id} | AbilityRead | handleGetFramework |
| GET | /api/v1/frameworks/{id}/dockerfile | AbilityRead | handleGetFrameworkDockerfile |
| POST | /api/v1/ci/oidc-exchange | Public | handleCIOIDCExchange |
| GET | /api/v1/instance | Public | handleInstance |
| GET | /api/v1/instance/identity | AbilityRead | handleInstanceIdentity |
| POST | /api/v1/instance/confirm-same | AbilityRoot | handleConfirmSameInstance |
| GET | /api/v1/search | AbilityRead | handleSearch |
| GET | /api/v1/search/suggest | AbilityRead | handleSearchSuggest |
| GET | /api/v1/search/saved | AbilityRead | handleListSavedSearches |
| POST | /api/v1/search/saved | AbilityWrite | handleSaveSearch |
| DELETE | /api/v1/search/saved/{id} | AbilityWrite | handleDeleteSavedSearch |
| POST | /api/v1/search/usage | AbilityWrite | handleSearchUsage |
| GET | /api/v1/plugin-panels | AbilityRead | handleListPluginPanels |
| GET | /api/v1/plugins/{name}/panels/{panel}/data | AbilityRead | handlePluginPanelData |
| POST | /api/v1/plugins/{name}/panels/{panel}/actions/{action} | AbilityWriteSensitive | handlePluginPanelAction |
| GET | /api/v1/plugin-marketplace | AbilityRoot | handleMarketplaceCatalog |
| POST | /api/v1/plugin-marketplace/refresh | AbilityRoot | handleMarketplaceRefresh |
| POST | /api/v1/plugin-marketplace/preview | AbilityRoot | handleMarketplacePreview |
| POST | /api/v1/plugin-marketplace/install | AbilityRoot | handleMarketplaceInstall |
| GET | /api/v1/plugin-marketplace/sources | AbilityRoot | handleMarketplaceSources |
| POST | /api/v1/plugin-marketplace/sources | AbilityRoot | handleMarketplaceAddSource |
| DELETE | /api/v1/plugin-marketplace/sources/{id} | AbilityRoot | handleMarketplaceRemoveSource |
| GET | /api/v1/resilience/disk | AbilityRead | handleGetDiskResilience |
| POST | /api/v1/resilience/disk/free-space | AbilityRoot | handleFreeDiskSpace |
| GET | /api/v1/alert-templates | AbilityRead | handleListAlertTemplates |
| GET | /api/v1/inbox | AbilityRead | handleListInbox |
| GET | /api/v1/inbox/unread-count | AbilityRead | handleInboxUnreadCount |
| POST | /api/v1/inbox/read | AbilityWrite | handleInboxMarkRead |
| GET | /api/v1/inbox/preferences | AbilityRead | handleGetInboxPrefs |
| PUT | /api/v1/inbox/preferences | AbilityWrite | handlePutInboxPrefs |
| POST | /api/v1/images/signature-policy/validate | AbilityWrite | handleValidateImageSignaturePolicy |
| POST | /api/v1/plugins/{name}/test | AbilityRoot | handleTestPlugin |
| POST | /api/v1/model-endpoints/test | AbilityWrite | handleTestEndpointInput |
| GET | /api/v1/migration/takeover/plan | AbilityRoot | handleTakeoverPlan |
| GET | /api/v1/migration/takeover/runs | AbilityRoot | handleListTakeoverRuns |
| POST | /api/v1/migration/takeover/runs | AbilityRoot | handleStartTakeover |
| GET | /api/v1/migration/takeover/runs/{run} | AbilityRoot | handleGetTakeoverRun |
| POST | /api/v1/migration/takeover/runs/{run}/rollback | AbilityRoot | handleRollbackTakeover |
| POST | /api/v1/migration/apps/ssh/host-keys | AbilityWriteSensitive | handleSSHHostKeys |
| POST | /api/v1/migration/apps/ssh/test | AbilityWriteSensitive | handleSSHTest |
| GET | /api/v1/migration/apps/sessions/{id}/volumes/copy | AbilityRead | handleAppImportVolumeCopyStatus |
| POST | /api/v1/migration/apps/sessions/{id}/volumes/copy | AbilityWriteSensitive | handleAppImportVolumeCopy |
| GET | /api/v1/sso/oidc | AbilityRoot | handleListOAuthSSOPolicies |
| GET | /api/v1/sso/oidc/{provider} | AbilityRoot | handleGetOAuthSSOPolicy |
| PUT | /api/v1/sso/oidc/{provider} | AbilityRoot | handlePutOAuthSSOPolicy |
| POST | /api/v1/sso/oidc/{provider}/evaluate | AbilityRoot | handleEvaluateOAuthSSO |
| GET | /api/v1/sso/oidc/{provider}/test/start | AbilityRoot | handleStartOAuthSSOTest |
| GET | /api/v1/sso/oidc/{provider}/test/result | AbilityRoot | handleOAuthSSOTestResult |
| GET | /api/v1/local-mode | AbilityRead | handleGetLocalMode |
| POST | /api/v1/local-mode/reset | AbilityRoot | handleLocalModeReset |
| GET | /api/v1/git-connections | AbilityReadSensitive | handleListGitConnections |
| POST | /api/v1/git-connections | AbilityRoot | handleCreateGitConnection |
| GET | /api/v1/git-connections/{id} | AbilityReadSensitive | handleGetGitConnection |
| PATCH | /api/v1/git-connections/{id} | AbilityRoot | handleRenameGitConnection |
| DELETE | /api/v1/git-connections/{id} | AbilityRoot | handleDisconnectGitConnection |
| POST | /api/v1/git-connections/{id}/default | AbilityRoot | handleSetDefaultGitConnection |
| GET | /api/v1/git-connections/{id}/branches | AbilityReadSensitive | handleListGitConnectionBranches |
| POST | /api/v1/git-connections/{id}/use-as-source | AbilityWriteSensitive | handleUseGitConnectionRepoAsSource |
| GET | /api/v1/diagnose/{kind}/{name} | AbilityRead | handleDiagnoseStuck |
| GET | /api/v1/alert-defaults | AbilityRead | handleListDefaultAlerts |
| PUT | /api/v1/alert-defaults/{id} | AbilityWrite | handleSetDefaultAlertEnabled |
| GET | /api/v1/go-live/status | AbilityRead | handleLiveBadges |
| GET | /api/v1/sso/saml/connections/{id}/test/start | AbilityRoot | handleStartSAMLTest |
| GET | /api/v1/sso/saml/connections/{id}/test/result | AbilityRoot | handleSAMLTestResult |
| GET | /api/v1/forgejo-app/connect | AbilityRoot | handleStartGitConnect |
| GET | /api/v1/forgejo-app/callback | AbilityRoot | handleGitConnectCallback |
| POST | /api/v1/hooks/deploy/{id}/{token} | Public | handleBuildHookTrigger |
| POST | /api/v1/forms/{app}/{name} | Public | handleFormSubmit |
| OPTIONS | /api/v1/forms/{app}/{name} | Public | handleFormPreflight |
| GET | /api/v1/mail/tokens | AbilityRead | handleListMailTokens |
| GET | /api/v1/mail/templates | AbilityRead | handleListMailTemplates |
| GET | /api/v1/mail/templates/{key} | AbilityRead | handleGetMailTemplate |
| PUT | /api/v1/mail/templates/{key} | AbilityRoot | handleSetMailTemplate |
| DELETE | /api/v1/mail/templates/{key} | AbilityRoot | handleResetMailTemplate |
| GET | /api/v1/mail/templates/{key}/versions | AbilityRead | handleListMailTemplateVersions |
| POST | /api/v1/mail/templates/{key}/versions/{version}/restore | AbilityRoot | handleRestoreMailTemplateVersion |
| POST | /api/v1/mail/templates/{key}/preview | AbilityRoot | handlePreviewMailTemplate |
| POST | /api/v1/mail/templates/{key}/test | AbilityRoot | handleTestMailTemplate |
| PUT | /api/v1/mail/layout | AbilityRoot | handleSetMailLayout |
| GET | /api/v1/mail/audience-options | AbilityRoot | handleMailAudienceOptions |
| POST | /api/v1/mail/audience/preview | AbilityRoot | handlePreviewMailAudience |
| GET | /api/v1/mail/groups | AbilityRoot | handleListMailGroups |
| POST | /api/v1/mail/groups | AbilityRoot | handleCreateMailGroup |
| GET | /api/v1/mail/groups/{id} | AbilityRoot | handleGetMailGroup |
| PUT | /api/v1/mail/groups/{id} | AbilityRoot | handleUpdateMailGroup |
| DELETE | /api/v1/mail/groups/{id} | AbilityRoot | handleDeleteMailGroup |
| GET | /api/v1/mail/suppressions | AbilityRoot | handleListMailSuppressions |
| POST | /api/v1/mail/suppressions | AbilityRoot | handleAddMailSuppression |
| DELETE | /api/v1/mail/suppressions/{email} | AbilityRoot | handleRemoveMailSuppression |
| GET | /api/v1/mail/broadcasts | AbilityRoot | handleListMailBroadcasts |
| POST | /api/v1/mail/broadcasts | AbilityRoot | handleCreateMailBroadcast |
| POST | /api/v1/mail/broadcasts/test | AbilityRoot | handleTestMailBroadcast |
| GET | /api/v1/mail/broadcasts/{id} | AbilityRoot | handleGetMailBroadcast |
| POST | /api/v1/mail/broadcasts/{id}/cancel | AbilityRoot | handleCancelMailBroadcast |
| POST | /api/v1/alert-defaults/reset | AbilityWrite | handleResetDefaultAlerts |
| GET | /api/v1/inbox/state | AbilityRead | handleInboxState |
| POST | /api/v1/inbox/archive | AbilityWrite | handleInboxArchive |
| POST | /api/v1/git-connections/test | AbilityRoot | handleTestGitConnection |
| GET | /api/v1/silences | AbilityRead | handleListSilences |
| POST | /api/v1/silences | AbilityWrite | handleCreateSilence |
| DELETE | /api/v1/silences/{id} | AbilityWrite | handleDeleteSilence |
| GET | /api/v1/functions/{name}/invocations | AbilityRead | handleListFunctionInvocations |
| POST | /api/v1/functions/{name}/invoke | AbilityDeploy | handleInvokeFunction |
| POST | /api/v1/functions/{name}/invocations/{id}/replay | AbilityDeploy | handleReplayFunctionInvocation |
| GET | /api/v1/functions/{name}/capture | AbilityRead | handleGetFunctionCapture |
| PUT | /api/v1/functions/{name}/capture | AbilityDeploy | handleSetFunctionCapture |
| GET | /api/v1/functions/{name}/settings | AbilityRead | handleGetFunctionSettings |
| PUT | /api/v1/functions/{name}/settings | AbilityDeploy | handlePutFunctionSettings |
| GET | /api/v1/functions/{name}/versions | AbilityRead | handleListFunctionVersions |
| POST | /api/v1/functions/{name}/rollback | AbilityDeploy | handleFunctionRollback |
| GET | /api/v1/functions/{name}/triggers | AbilityRead | handleGetFunctionTriggers |
| PUT | /api/v1/functions/{name}/triggers/webhook | AbilityDeploy | handlePutFunctionWebhook |
| DELETE | /api/v1/functions/{name}/triggers/webhook | AbilityDeploy | handleDeleteFunctionWebhook |
| POST | /api/v1/functions/{name}/async | AbilityDeploy | handleEnqueueFunctionInvocation |
| GET | /api/v1/function-invocations | AbilityRead | handleListQueuedFunctionCalls |
| POST | /api/v1/function-invocations/{id}/retry | AbilityDeploy | handleRetryFunctionInvocation |
| DELETE | /api/v1/function-invocations/{id} | AbilityDeploy | handleDeleteFunctionInvocation |
| POST | /api/v1/migration/agent/sessions | AbilityWriteSensitive | handleCreateMigrationAgentSession |
| GET | /api/v1/migration/agent/sessions | AbilityRead | handleListMigrationAgentSessions |
| GET | /api/v1/migration/agent/sessions/{id} | AbilityWriteSensitive | handleGetMigrationAgentSession |
| POST | /api/v1/migration/agent/sessions/{id}/discover | AbilityWriteSensitive | handleDiscoverMigrationAgent |
| POST | /api/v1/migration/agent/sessions/{id}/verify | AbilityWriteSensitive | handleVerifyMigrationAgent |
| DELETE | /api/v1/migration/agent/sessions/{id} | AbilityWriteSensitive | handleRevokeMigrationAgentSession |
| GET | /api/v1/alert-defaults/cert-expiry/email | AbilityRead | handleGetCertExpiryEmail |
| PUT | /api/v1/alert-defaults/cert-expiry/email | AbilityWrite | handleSetCertExpiryEmail |
| POST | /api/v1/inbox/delete | AbilityWrite | handleInboxDelete |
| POST | /api/v1/inbox/clear | AbilityWrite | handleInboxClear |
| POST | /api/v1/git-sources/preview | AbilityWrite | handlePreviewRepo |
| GET | /api/v1/functions/starters | AbilityRead | handleFunctionStarters |
| GET | /api/v1/functions/{name}/kv | AbilityRead | handleListFunctionKV |
| GET | /api/v1/functions/{name}/kv/value | AbilityRead | handleGetFunctionKVValue |
| PUT | /api/v1/functions/{name}/kv/value | AbilityDeploy | handlePutFunctionKVValue |
| DELETE | /api/v1/functions/{name}/kv/value | AbilityDeploy | handleDeleteFunctionKVValue |
| POST | /api/v1/functions/{name}/kv/token | AbilityWriteSensitive | handleRotateFunctionKVToken |
| GET | /api/v1/function-kv/entry | Public | handleFunctionKVEntry |
| PUT | /api/v1/function-kv/entry | Public | handleFunctionKVEntry |
| DELETE | /api/v1/function-kv/entry | Public | handleFunctionKVEntry |
| GET | /api/v1/function-kv/keys | Public | handleFunctionKVKeys |
| POST | /api/v1/migration/coolify/exit-report | AbilityWriteSensitive | handleCoolifyExitReport |
| GET | /api/v1/migration/apps/sessions/{id}/exit-report | AbilityRead | handleAppImportExitReport |
| POST | /api/v1/migration/apps/test | AbilityWriteSensitive | handleAppImportTest |
| GET | /api/v1/migration/apps/sessions/{id}/progress | AbilityRead | handleAppImportProgress |
| GET | /api/v1/migration/apps/sessions/{id}/events | AbilityRead | handleAppImportEvents |
| POST | /api/v1/migration/apps/sessions/{id}/probe | AbilityWriteSensitive | handleAppImportProbe |
| GET | /api/v1/migration/apps/sessions/{id}/retire | AbilityRead | handleAppImportRetire |
| POST | /api/v1/migration/apps/sessions/{id}/retire | AbilityWriteSensitive | handleAppImportRetire |
| POST | /api/v1/migration/exit-report | AbilityWriteSensitive | handleCoolifyExitReport |
| GET | /api/v1/deployments/{id}/timeline | AbilityRead | handleGetDeployTimeline |
| GET | /api/v1/deployments/{id}/timeline/stream | AbilityRead | handleDeployTimelineStream |
| GET | /api/v1/deployments/{id}/timeline/compare | AbilityRead | handleCompareDeployTimelines |
| GET | /api/v1/benchmarks | AbilityRead | handleListBenchmarks |
| POST | /api/v1/benchmarks | AbilityWrite | handleUploadBenchmark |
| GET | /api/v1/benchmarks/compare | AbilityRead | handleCompareBenchmarks |
| GET | /api/v1/benchmarks/{id} | AbilityRead | handleGetBenchmark |
| DELETE | /api/v1/benchmarks/{id} | AbilityRoot | handleDeleteBenchmark |
| GET | /api/v1/costs/app-caps | AbilityRead | handleListSpendCaps |
| POST | /api/v1/costs/budgets/{id}/dry-run | AbilityRead | handleCostBudgetDryRun |
| POST | /api/v1/migrations/verify | AbilityRoot | handleMigrationVerify |
| GET | /api/v1/migrations/verify/latest | AbilityRead | handleLatestMigrationVerify |
| POST | /api/v1/imports/repo/plan | AbilityWrite | handleImportRepoPlan |
| POST | /api/v1/imports/repo/apply | AbilityWrite | handleImportRepoApply |
| GET | /api/v1/cleanup | AbilityRead | handleGetCleanup |
| GET | /api/v1/cleanup/runs | AbilityRead | handleListCleanupRuns |
| POST | /api/v1/cleanup/{id}/preview | AbilityRoot | handlePreviewCleanup |
| POST | /api/v1/cleanup/{id}/run | AbilityRoot | handleRunCleanup |
| PUT | /api/v1/cleanup/{id}/mode | AbilityRoot | handleSetCleanupMode |
| POST | /api/v1/imports/platform/databases/{name}/hosted/preflight | AbilityWriteSensitive | handleHostedPreflight |
| POST | /api/v1/imports/platform/databases/{name}/hosted/run | AbilityWriteSensitive | handleHostedRun |
| GET | /api/v1/stacks | AbilityRead | handleListStacks |
| POST | /api/v1/stacks | AbilityWrite | handleCreateStack |
| GET | /api/v1/stacks/{name} | AbilityRead | handleGetStack |
| PATCH | /api/v1/stacks/{name} | AbilityRoot | handleUpdateStackSettings |
| DELETE | /api/v1/stacks/{name} | AbilityWrite | handleDeleteStack |
| POST | /api/v1/stacks/{name}/plan | AbilityRead | handleStackPlan |
| POST | /api/v1/stacks/{name}/apply | AbilityWrite | handleStackApply |
| GET | /api/v1/stacks/{name}/revisions | AbilityRead | handleStackRevisions |
| GET | /api/v1/stacks/{name}/revisions/{n} | AbilityRead | handleStackRevision |
| GET | /api/v1/stacks/{name}/revisions/{n}/diff | AbilityRead | handleStackDiff |
| POST | /api/v1/stacks/{name}/revisions/{n}/rollback | AbilityWrite | handleStackRollback |
| PUT | /api/v1/stacks/{name}/pin | AbilityWrite | handleStackPin |
| DELETE | /api/v1/stacks/{name}/pin | AbilityWrite | handleStackUnpin |
| POST | /api/v1/stacks/{name}/unlock | AbilityRoot | handleStackUnlock |
| POST | /api/v1/stacks/{name}/revisions/{n}/approve | AbilityRoot | handleStackApprove |
| POST | /api/v1/stacks/{name}/revisions/{n}/reject | AbilityRoot | handleStackReject |
| POST | /api/v1/stacks/{name}/oidc-trusts | AbilityRoot | handleCreateStackTrust |
| GET | /api/v1/stacks/{name}/oidc-trusts | AbilityRoot | handleListStackTrusts |
| DELETE | /api/v1/stacks/{name}/oidc-trusts/{id} | AbilityRoot | handleDeleteStackTrust |
| POST | /api/v1/ci/stack-oidc-exchange | Public | handleStackOIDCExchange |
| GET | /api/v1/assets/buckets | AbilityRead | handleListAssetBuckets |
| POST | /api/v1/assets/buckets | AbilityWriteSensitive | handleCreateAssetBucket |
| GET | /api/v1/assets/buckets/{id} | AbilityRead | handleGetAssetBucket |
| PATCH | /api/v1/assets/buckets/{id} | AbilityWriteSensitive | handleUpdateAssetBucket |
| DELETE | /api/v1/assets/buckets/{id} | AbilityWriteSensitive | handleDeleteAssetBucket |
| POST | /api/v1/assets/buckets/{id}/probe | AbilityWriteSensitive | handleProbeAssetBucket |
| GET | /api/v1/assets/buckets/{id}/usage | AbilityRead | handleAssetUsage |
| GET | /api/v1/assets/buckets/{id}/assets | AbilityReadSensitive | handleListAssets |
| GET | /api/v1/assets/buckets/{id}/urls | AbilityReadSensitive | handleAssetURLs |
| POST | /api/v1/assets/buckets/{id}/uploads | AbilityWrite | handleRequestAssetUpload |
| POST | /api/v1/assets/buckets/{id}/uploads/confirm | AbilityWrite | handleConfirmAssetUpload |
| PUT | /api/v1/assets/buckets/{id}/objects | AbilityWrite | handleUploadAsset |
| DELETE | /api/v1/assets/buckets/{id}/objects | AbilityWrite | handleDeleteAsset |
| GET | /api/v1/stack-templates | AbilityRead | handleListStackTemplates |
| GET | /api/v1/stack-templates/{id} | AbilityRead | handleGetStackTemplate |
| POST | /api/v1/stack-templates/{id}/render | AbilityWrite | handleRenderStackTemplate |
| POST | /api/v1/import/k8s | AbilityWrite | handleImportK8s |

## See also

- [Feature Catalog](/feature-catalog) - high-level overview of platform capabilities
- [Observability](/observability) - metrics, logs, alerts, and telemetry APIs
- [Architecture](/architecture) - control plane design and reconciliation patterns
- [CLI Reference](/cli-reference) - command-line tool for all API operations
