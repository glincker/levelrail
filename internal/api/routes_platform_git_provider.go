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

	// GitLab, Bitbucket and Gitea connections: many per provider. Listing and
	// reading are AbilityReadSensitive like the repository lists they feed;
	// creating, renaming, defaulting and disconnecting are AbilityRoot because a
	// connection grants read access to private repositories control-plane wide.
	// connect and callback are full-page browser navigations, so the session
	// cookie is implicitly root.
	mux.HandleFunc("POST /api/v1/git-connections/test", rt.requireAbility(AbilityRoot, rt.handleTestGitConnection))
	mux.HandleFunc("GET /api/v1/git-connections", rt.requireAbility(AbilityReadSensitive, rt.handleListGitConnections))
	mux.HandleFunc("POST /api/v1/git-connections", rt.requireAbility(AbilityRoot, rt.handleCreateGitConnection))
	mux.HandleFunc("GET /api/v1/git-connections/{id}", rt.requireAbility(AbilityReadSensitive, rt.handleGetGitConnection))
	mux.HandleFunc("PATCH /api/v1/git-connections/{id}", rt.requireAbility(AbilityRoot, rt.handleRenameGitConnection))
	mux.HandleFunc("DELETE /api/v1/git-connections/{id}", rt.requireAbility(AbilityRoot, rt.handleDisconnectGitConnection))
	mux.HandleFunc("POST /api/v1/git-connections/{id}/default", rt.requireAbility(AbilityRoot, rt.handleSetDefaultGitConnection))
	mux.HandleFunc("GET /api/v1/git-connections/{id}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGitConnectionBranches))
	mux.HandleFunc("POST /api/v1/git-connections/{id}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseGitConnectionRepoAsSource))
	mux.HandleFunc("GET /api/v1/gitlab-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGitConnect(gitProviderGitLab)))
	mux.HandleFunc("GET /api/v1/gitlab-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitConnectCallback(gitProviderGitLab)))
	mux.HandleFunc("GET /api/v1/bitbucket-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGitConnect(gitProviderBitbucket)))
	mux.HandleFunc("GET /api/v1/bitbucket-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitConnectCallback(gitProviderBitbucket)))
	mux.HandleFunc("GET /api/v1/gitea-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGitConnect(gitProviderGitea)))
	mux.HandleFunc("GET /api/v1/gitea-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitConnectCallback(gitProviderGitea)))
	mux.HandleFunc("GET /api/v1/forgejo-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGitConnect(gitProviderForgejo)))
	mux.HandleFunc("GET /api/v1/forgejo-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitConnectCallback(gitProviderForgejo)))
}
