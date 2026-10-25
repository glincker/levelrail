package api

import "net/http"

// registerAuthRoutes registers the sessions, users, roles, invites, IAM, devices, OAuth and tokens routes.
func (rt *Router) registerAuthRoutes(mux *http.ServeMux) {
	// Auth. Login and first-run registration are necessarily public;
	// everything else requires an existing session.
	mux.HandleFunc("POST /api/v1/auth/login", rt.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/register", rt.handleRegister)
	mux.HandleFunc("GET /api/v1/auth/setup-status", rt.handleSetupStatus)
	mux.HandleFunc("POST /api/v1/auth/logout", rt.requireSessionOrPinned(rt.handleLogout))
	mux.HandleFunc("PUT /api/v1/auth/password", rt.requireAuth(rt.handleChangePassword))
	mux.HandleFunc("GET /api/v1/auth/session", rt.handleAuthSession)
	mux.HandleFunc("GET /api/v1/auth/whoami", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleWhoami))
	mux.HandleFunc("PATCH /api/v1/auth/profile", rt.requireAuth(rt.handleUpdateProfile))
	mux.HandleFunc("GET /api/v1/auth/my-access", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleMyAccess))
	mux.HandleFunc("POST /api/v1/auth/sessions/revoke-others", rt.requireAuth(rt.handleRevokeOtherSessions))

	// Sign in with a code and new-device approval (login_code.go,
	// login_approval.go). Request, redeem and poll are public and bound to a
	// browser cookie; the rest act only on the caller's own account.
	mux.HandleFunc("GET /api/v1/auth/login-options", rt.handleLoginOptions)
	mux.HandleFunc("POST /api/v1/auth/login-code/request", rt.handleRequestLoginCode)
	mux.HandleFunc("POST /api/v1/auth/login-code/redeem", rt.handleRedeemLoginCode)
	mux.HandleFunc("POST /api/v1/auth/login-approval/poll", rt.handlePollLoginApproval)
	mux.HandleFunc("GET /api/v1/auth/sign-in-requests", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleListSignInRequests))
	mux.HandleFunc("POST /api/v1/auth/sign-in-requests/codes/{id}/reveal", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.writeBudget(rt.handleRevealLoginCode)))
	mux.HandleFunc("POST /api/v1/auth/login-approvals/{id}/approve", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.writeBudget(rt.handleApproveLoginApproval)))
	mux.HandleFunc("POST /api/v1/auth/login-approvals/{id}/deny", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.writeBudget(rt.handleDenyLoginApproval)))
	mux.HandleFunc("GET /api/v1/auth/trusted-devices", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleListTrustedDevices))
	mux.HandleFunc("DELETE /api/v1/auth/trusted-devices/{id}", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.writeBudget(rt.handleRevokeTrustedDevice)))
	mux.HandleFunc("POST /api/v1/auth/cli-pairing", rt.requireAuth(rt.writeBudget(rt.handleCreateCLIPairing)))
	mux.HandleFunc("POST /api/v1/auth/cli-pairing/redeem", rt.handleRedeemCLIPairing)
	mux.HandleFunc("GET /api/v1/settings/auth/code-login", rt.requireAbility(AbilityRead, rt.handleGetCodeLoginSettings))
	mux.HandleFunc("PUT /api/v1/settings/auth/code-login", rt.requireAbility(AbilityRoot, rt.handlePutCodeLoginSettings))

	// Session links (session_links.go): a short-lived, single-use,
	// URL-embeddable token for browser automation to skip manual login.
	// Minting is root-equivalent since the resulting session inherits
	// the minting caller's own abilities; consuming is necessarily
	// unauthenticated, gated by possession of the token itself, same
	// shape as reset-password below.
	mux.HandleFunc("POST /api/v1/auth/session-links", rt.requireAbility(AbilityRoot, rt.handleMintSessionLink))
	mux.HandleFunc("POST /api/v1/auth/session-links/consume", rt.handleConsumeSessionLink)
	mux.HandleFunc("GET /api/v1/auth/session-links/{token}/consume", rt.handleConsumeSessionLinkGET)

	// Two-factor auth (twofactor.go). /2fa/verify is necessarily public
	// (it's step two of login, before a session exists), rate-limited by
	// rt.mfaVerify instead of a session/ability check. Every other route
	// here acts on the caller's own account, so requireAuth
	// (session-only, matching handleChangePassword's own reasoning).
	mux.HandleFunc("GET /api/v1/auth/2fa", rt.requireAuth(rt.mfaLib.status))
	mux.HandleFunc("POST /api/v1/auth/2fa/setup", rt.requireAuth(rt.mfaLib.setup))
	mux.HandleFunc("POST /api/v1/auth/2fa/confirm", rt.requireAuth(rt.mfaLib.confirm))
	mux.HandleFunc("POST /api/v1/auth/2fa/disable", rt.requireAuth(rt.mfaLib.disable))
	mux.HandleFunc("POST /api/v1/auth/2fa/recovery-codes/regenerate", rt.requireAuth(rt.mfaLib.regenerate))
	mux.HandleFunc("POST /api/v1/auth/2fa/verify", rt.mfaLib.verify)

	// Passkeys (WebAuthn, passkeys.go). Registration acts on the
	// caller's own account, so requireAuth like the 2FA routes above.
	// /passkey-login/... is the sign-in ceremony, necessarily public,
	// same shape as /2fa/verify.
	mux.HandleFunc("GET /api/v1/auth/passkeys", rt.requireAuth(rt.mfaLib.listPasskeys))
	mux.HandleFunc("POST /api/v1/auth/passkeys/register/begin", rt.requireAuth(rt.mfaLib.beginRegistration))
	mux.HandleFunc("POST /api/v1/auth/passkeys/register/finish", rt.requireAuth(rt.mfaLib.finishRegistration))
	mux.HandleFunc("DELETE /api/v1/auth/passkeys/{id}", rt.requireAuth(rt.mfaLib.deletePasskey))
	mux.HandleFunc("PATCH /api/v1/auth/passkeys/{id}", rt.requireAuth(rt.mfaLib.renamePasskey))
	mux.HandleFunc("POST /api/v1/auth/passkey-login/begin", rt.mfaLib.beginLogin)
	mux.HandleFunc("POST /api/v1/auth/passkey-login/finish", rt.mfaLib.finishLogin)

	// Browser push notification subscriptions (push_subscriptions.go):
	// one admin account's registered browsers, the delivery target for
	// the "webpush" notification-channel kind. Self-service like
	// passkeys above, so requireAuth not requireAbility.
	mux.HandleFunc("GET /api/v1/settings/push-subscriptions/vapid-public-key", rt.requireAuth(rt.handleGetPushVAPIDPublicKey))
	mux.HandleFunc("GET /api/v1/settings/push-subscriptions", rt.requireAuth(rt.handleListPushSubscriptions))
	mux.HandleFunc("POST /api/v1/settings/push-subscriptions", rt.requireAuth(rt.instanceMembersOnly(rt.handleCreatePushSubscription)))
	mux.HandleFunc("DELETE /api/v1/settings/push-subscriptions/{id}", rt.requireAuth(rt.handleDeletePushSubscription))

	// Multi-user: creating another local-password user (see
	// handleRegister's own doc comment) is AbilityRoot, not merely
	// requireAuth: the caller also picks the new user's Abilities, so
	// anyone able to reach this route can mint access at any tier,
	// themselves included, only a root caller may do that. Listing stays
	// AbilityRead, same tier as every other passive view.
	mux.HandleFunc("POST /api/v1/auth/users", rt.requireAbility(AbilityRoot, rt.handleCreateUser))
	mux.HandleFunc("GET /api/v1/users", rt.requireAbility(AbilityRead, rt.handleListUsers))
	mux.HandleFunc("DELETE /api/v1/users/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteUser))
	// PUT .../abilities is AbilityRoot like the delete route above, and
	// refuses self-edits inside the handler (self-lockout guard): a root
	// caller may change any other user's abilities, never their own.
	mux.HandleFunc("PUT /api/v1/users/{id}/abilities", rt.requireAbility(AbilityRoot, rt.handleUpdateUserAbilities))
	// Curated role presets (roles.go): static, non-sensitive metadata,
	// AbilityRead like the user list itself, so any signed-in caller can
	// populate a role picker even without AbilityRoot.
	mux.HandleFunc("GET /api/v1/roles", rt.requireAbility(AbilityRead, rt.handleListRoles))
	// Stored roles, role assignment and environment grants: root, and the
	// handlers answer the experimental-disabled error while access-roles is off.
	mux.HandleFunc("POST /api/v1/roles", rt.requireAbility(AbilityRoot, rt.handleCreateRole))
	mux.HandleFunc("PUT /api/v1/roles/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateRole))
	mux.HandleFunc("DELETE /api/v1/roles/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteRole))
	mux.HandleFunc("PUT /api/v1/users/{id}/role", rt.requireAbility(AbilityRoot, rt.handleSetUserRole))
	mux.HandleFunc("PUT /api/v1/users/{id}/password-change-required", rt.requireAbility(AbilityRoot, rt.handleSetPasswordChangeRequired))
	mux.HandleFunc("GET /api/v1/users/{id}/environment-grants", rt.requireAbility(AbilityRoot, rt.handleGetEnvironmentGrants))
	mux.HandleFunc("PUT /api/v1/users/{id}/environment-grants", rt.requireAbility(AbilityRoot, rt.handlePutEnvironmentGrants))

	// Team invites: create/revoke are AbilityWrite because an invite can never carry more than
	// the caller holds; list is scoped to the caller's own invites; accept is public, gated by the emailed token.
	mux.HandleFunc("POST /api/v1/invites", rt.requireAbility(AbilityWrite, rt.handleCreateInvite))
	mux.HandleFunc("GET /api/v1/invites", rt.requireAbility(AbilityRead, rt.handleListInvites))
	mux.HandleFunc("DELETE /api/v1/invites/{id}", rt.requireAbility(AbilityWrite, rt.handleRevokeInvite))
	mux.HandleFunc("POST /api/v1/invites/accept", rt.handleAcceptInvite)

	// IAM policies (iam.go/iam_handlers.go): resource-scoped Allow/Deny
	// documents attached to a user or token, additive on top of the flat
	// Abilities list above. Reading the catalog is AbilityRead like roles
	// above; every mutation (create/update/delete/attach/detach) is
	// AbilityRoot, the same tier as handleUpdateUserAbilities, since a
	// policy can grant or deny access at a resource-scoped level a
	// non-root caller could not otherwise touch.
	mux.HandleFunc("POST /api/v1/iam/policies", rt.requireAbility(AbilityRoot, rt.handleCreatePolicy))
	mux.HandleFunc("GET /api/v1/iam/policies", rt.requireAbility(AbilityRead, rt.handleListPolicies))
	mux.HandleFunc("GET /api/v1/iam/policies/{id}", rt.requireAbility(AbilityRead, rt.handleGetPolicy))
	mux.HandleFunc("PUT /api/v1/iam/policies/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdatePolicy))
	mux.HandleFunc("DELETE /api/v1/iam/policies/{id}", rt.requireAbility(AbilityRoot, rt.handleDeletePolicy))
	mux.HandleFunc("GET /api/v1/iam/policies/{id}/attachments", rt.requireAbility(AbilityRead, rt.handleListPolicyAttachments))
	mux.HandleFunc("POST /api/v1/iam/policies/{id}/attachments", rt.requireAbility(AbilityRoot, rt.handleAttachPolicy))
	mux.HandleFunc("DELETE /api/v1/iam/policies/{id}/attachments/{principal_type}/{principal_id}", rt.requireAbility(AbilityRoot, rt.handleDetachPolicy))

	// CLI device login (device_auth.go): "levelrail-cli auth login
	// --device" prints a code, the operator approves it here. start/token
	// are necessarily public (no credential exists yet); requests/
	// approve/deny are requireAuth session-only, the same tier
	// tokens.go's own session-only routes use, since approving a device
	// only ever mints a token scoped to the approving operator's own
	// abilities.
	mux.HandleFunc("GET /api/v1/instance", rt.handleInstance)
	mux.HandleFunc("GET /api/v1/instance/identity", rt.requireAbility(AbilityRead, rt.handleInstanceIdentity))
	mux.HandleFunc("POST /api/v1/instance/confirm-same", rt.requireAbility(AbilityRoot, rt.handleConfirmSameInstance))
	mux.HandleFunc("POST /api/v1/auth/device/start", rt.handleDeviceAuthStart)
	mux.HandleFunc("POST /api/v1/auth/device/token", rt.handleDeviceAuthToken)
	mux.HandleFunc("GET /api/v1/auth/device/requests", rt.requireAuth(rt.handleListDeviceAuthRequests))
	mux.HandleFunc("GET /api/v1/auth/device/pending-summary", rt.requireAbility(AbilityRead, rt.handleDevicePendingSummary))
	mux.HandleFunc("GET /api/v1/auth/device/activity", rt.requireAbility(AbilityRead, rt.handleDeviceActivity))
	mux.HandleFunc("GET /api/v1/attention/feed", rt.requireAbility(AbilityRead, rt.handleAttentionFeed))
	mux.HandleFunc("POST /api/v1/attention/dismiss", rt.requireAuth(rt.handleAttentionDismiss))
	mux.HandleFunc("POST /api/v1/auth/device/{user_code}/approve", rt.requireAuth(rt.handleApproveDeviceAuthRequest))
	mux.HandleFunc("POST /api/v1/auth/device/{user_code}/deny", rt.requireAuth(rt.handleDenyDeviceAuthRequest))

	// OAuth sign-in (Google, GitHub). /providers, /start, /callback are
	// all necessarily public; /link/start is requireAuth-gated (see its
	// own doc comment).
	mux.HandleFunc("GET /api/v1/auth/oauth/providers", rt.handleListPublicOAuthProviders)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", rt.handleOAuthStart)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", rt.handleOAuthCallback)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/link/start", rt.requireAuth(rt.handleOAuthLinkStart))

	// OAuth settings: GET is AbilityRead, PUT is AbilityRoot, matching
	// /api/v1/settings/ingress's own tiers.
	// Global deploy freeze windows apply to every app, so writing them is root-only.
	mux.HandleFunc("GET /api/v1/settings/deploy-freeze", rt.requireAbility(AbilityRead, rt.handleGetGlobalDeployFreeze))
	mux.HandleFunc("PUT /api/v1/settings/deploy-freeze", rt.requireAbility(AbilityRoot, rt.handlePutGlobalDeployFreeze))
	mux.HandleFunc("GET /api/v1/settings/oauth", rt.requireAbility(AbilityRead, rt.handleListOAuthSettings))
	mux.HandleFunc("PUT /api/v1/settings/oauth/{provider}", rt.requireAbility(AbilityRoot, rt.handleUpdateOAuthProviderSettings))

	// Forgot/reset password: necessarily unauthenticated, gated by
	// possession of the emailed token instead of a session or ability.
	mux.HandleFunc("POST /api/v1/auth/forgot-password", rt.handleForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", rt.handleResetPassword)

	// API tokens: create/revoke are session-only, deliberately never
	// bearer-token authenticated (list also takes a root token). A token cannot mint or revoke another token on its
	// own behalf; only an interactive human session can manage the
	// token set, the same boundary that stops a leaked scoped token from
	// escalating itself by minting a broader one.
	mux.HandleFunc("POST /api/v1/auth/tokens", rt.requireAuth(rt.handleCreateToken))
	mux.HandleFunc("GET /api/v1/auth/tokens", rt.sessionOrRootToken(rt.handleListTokens))
	mux.HandleFunc("DELETE /api/v1/auth/tokens/{id}", rt.requireAuth(rt.handleRevokeToken))
	// Removing revoked tokens is irreversible bookkeeping on the token set, so AbilityRoot.
	mux.HandleFunc("POST /api/v1/auth/tokens/cleanup", rt.requireAbility(AbilityRoot, rt.handleCleanupTokens))
}

// handleAuthSession answers GET /api/v1/auth/session. Pinned (token-minted)
// sessions have no user row and cannot pass requireAuth, so they skip it.
func (rt *Router) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.currentPinnedSession(r); ok {
		rt.handleGetSession(w, r)
		return
	}
	rt.requireAuth(rt.handleGetSession)(w, r)
}
