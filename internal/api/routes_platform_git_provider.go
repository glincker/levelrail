package api

import "net/http"

// registerGitProviderRoutes registers the git provider apps routes.
func (rt *Router) registerGitProviderRoutes(mux *http.ServeMux) {
	// Aggregated git provider capability summary (git_providers.go): one
	// AbilityReadSensitive call the git-source picker uses instead of the
	// three AbilityRoot status endpoints below, so a non-root deploy-scoped
	// user can open the app creation wizard without hitting a 403. See
	// handleListGitProviders's own doc comment.
	mux.HandleFunc("GET /api/v1/git-providers", rt.requireAbility(AbilityReadSensitive, rt.handleListGitProviders))

	// One searchable, filterable, paged repository listing for every provider
	// (git_repo_list.go); the per-provider listings below share its cache.
	mux.HandleFunc("GET /api/v1/git/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListGitRepos))

	// GitHub App connection: routes that read or change the connection are AbilityRoot, since an
	// installed app can read private repos. register/start, callback and installed are full-page
	// browser navigations; the session cookie is implicitly root.
	//
	// Repo and branch listing is AbilityReadSensitive; use-as-source is AbilityWriteSensitive like git-source.
	mux.HandleFunc("GET /api/v1/github-app", rt.requireAbility(AbilityRoot, rt.handleGetGitHubAppStatus))
	mux.HandleFunc("DELETE /api/v1/github-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectGitHubApp))
	mux.HandleFunc("PUT /api/v1/github-app/manual", rt.requireAbility(AbilityRoot, rt.handleConnectGitHubAppManually))
	mux.HandleFunc("GET /api/v1/github-app/register/preview", rt.requireAbility(AbilityRoot, rt.handleGetGitHubAppManifestPreview))
	mux.HandleFunc("GET /api/v1/github-app/register/start", rt.requireAbility(AbilityRoot, rt.handleStartGitHubAppRegistration))
	mux.HandleFunc("GET /api/v1/github-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitHubAppCallback))
	mux.HandleFunc("GET /api/v1/github-app/installed", rt.requireAbility(AbilityRoot, rt.handleGitHubAppInstalled))
	mux.HandleFunc("GET /api/v1/github-app/installations", rt.requireAbility(AbilityRoot, rt.handleListGitHubAppInstallations))
	mux.HandleFunc("DELETE /api/v1/github-app/installations/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteGitHubAppInstallation))
	mux.HandleFunc("GET /api/v1/github-app/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListGitHubAppRepos))
	mux.HandleFunc("GET /api/v1/github-app/repos/{owner}/{repo}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGitHubAppBranches))
	mux.HandleFunc("POST /api/v1/github-app/repos/{owner}/{repo}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseGitHubRepoAsSource))

	// GitLab App: the OAuth-Application counterpart of the GitHub App
	// routes above, same ability tiers for the same reasons. connect and
	// callback are real, full-page browser navigations (GitLab's OAuth2
	// authorization endpoint requires that), not fetch calls.
	// use-as-source is AbilityWriteSensitive, matching PUT
	// .../git-source's own tier: it performs that exact action.
	mux.HandleFunc("GET /api/v1/gitlab-app", rt.requireAbility(AbilityRoot, rt.handleGetGitLabAppStatus))
	mux.HandleFunc("PUT /api/v1/gitlab-app", rt.requireAbility(AbilityRoot, rt.handleConnectGitLabApp))
	mux.HandleFunc("DELETE /api/v1/gitlab-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectGitLabApp))
	mux.HandleFunc("GET /api/v1/gitlab-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGitLabAppConnect))
	mux.HandleFunc("GET /api/v1/gitlab-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitLabAppCallback))
	mux.HandleFunc("GET /api/v1/gitlab-app/projects", rt.requireAbility(AbilityReadSensitive, rt.handleListGitLabAppProjects))
	mux.HandleFunc("GET /api/v1/gitlab-app/projects/{id}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGitLabAppBranches))
	mux.HandleFunc("POST /api/v1/gitlab-app/projects/{id}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseGitLabProjectAsSource))

	// Bitbucket App: the OAuth-consumer counterpart of the GitLab App
	// routes above, same ability tiers for the same reasons, same
	// two-step "configure, then authorize" shape. Cloud only, no
	// instance_url (docs/design/git-provider-integrations.md section 3).
	mux.HandleFunc("GET /api/v1/bitbucket-app", rt.requireAbility(AbilityRoot, rt.handleGetBitbucketAppStatus))
	mux.HandleFunc("PUT /api/v1/bitbucket-app", rt.requireAbility(AbilityRoot, rt.handleConnectBitbucketApp))
	mux.HandleFunc("DELETE /api/v1/bitbucket-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectBitbucketApp))
	mux.HandleFunc("GET /api/v1/bitbucket-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartBitbucketAppConnect))
	mux.HandleFunc("GET /api/v1/bitbucket-app/callback", rt.requireAbility(AbilityRoot, rt.handleBitbucketAppCallback))
	mux.HandleFunc("GET /api/v1/bitbucket-app/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListBitbucketAppRepos))
	mux.HandleFunc("GET /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListBitbucketAppBranches))
	mux.HandleFunc("POST /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseBitbucketRepoAsSource))

	// Gitea App: the OAuth-Application counterpart of the GitLab App
	// routes above, same ability tiers and self-hosted instance_url shape
	// (Gitea is almost always self-hosted). Repos are addressed by an
	// "owner/repo" path pair like Bitbucket's workspace/repoSlug, not a
	// numeric id like GitLab's.
	mux.HandleFunc("GET /api/v1/gitea-app", rt.requireAbility(AbilityRoot, rt.handleGetGiteaAppStatus))
	mux.HandleFunc("PUT /api/v1/gitea-app", rt.requireAbility(AbilityRoot, rt.handleConnectGiteaApp))
	mux.HandleFunc("DELETE /api/v1/gitea-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectGiteaApp))
	mux.HandleFunc("GET /api/v1/gitea-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGiteaAppConnect))
	mux.HandleFunc("GET /api/v1/gitea-app/callback", rt.requireAbility(AbilityRoot, rt.handleGiteaAppCallback))
	mux.HandleFunc("GET /api/v1/gitea-app/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListGiteaAppRepos))
	mux.HandleFunc("GET /api/v1/gitea-app/repos/{owner}/{repo}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGiteaAppBranches))
	mux.HandleFunc("POST /api/v1/gitea-app/repos/{owner}/{repo}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseGiteaRepoAsSource))
}
