// Wire types for the GitHub App connection feature
// (internal/api/github_app.go, github_app_repos.go), matching their
// resource structs field for field.

import type { GitSourceResource } from './gitSource'

// GitHubAppStatus mirrors gitHubAppStatusResource
// (internal/api/github_app.go). Deliberately no secret fields: the
// backend never echoes client_secret/webhook_secret/private_key back in
// any response body, so there is nothing to type here for them.
export interface GitHubAppStatus {
  connected: boolean
  app_id?: number
  client_id?: string
  // instance_url is "https://github.com" for every connection until a
  // GitHub Enterprise Server one is connected, empty when not connected.
  instance_url?: string
  installed: boolean
  // installation_status is set only when the backend actually ran a live
  // check against GitHub (an installation_id is on record and a private
  // key was available to sign the check with): 'installed' | 'suspended'
  // | 'not_found'. Empty when there's nothing to check yet, or the check
  // itself couldn't run, in which case installed falls back to the last
  // known local state.
  installation_status?: 'installed' | 'suspended' | 'not_found'
  account_login?: string
  created_at?: string
  // base_url is what handleStartGitHubAppRegistration's manifest and
  // webhook/callback/setup URLs will actually point at (derived from
  // ingress settings' primary domain), empty when no primary domain is
  // configured yet. Shown as a disclaimer before the automated flow, so
  // connecting never surprises an operator with a callback pointing
  // somewhere other than where they're actually running this instance.
  base_url?: string
}

// GitHubAppManualConnectRequest mirrors manualGitHubAppRequest
// (internal/api/github_app_manual.go): every field GitHub's own App
// settings page shows for an App created by hand, typed in instead of
// exchanged via the manifest flow.
export interface GitHubAppManualConnectRequest {
  app_id: number
  client_id: string
  // instance_url is optional: omitted or "" means github.com, matching
  // the backend's own normalizeGitHubInstanceURL default.
  instance_url?: string
  client_secret: string
  webhook_secret: string
  private_key: string
  installation_id?: number
  account_login?: string
}

// GitHubAppRepo mirrors gitHubAppRepoResource
// (internal/api/github_app_repos.go).
export interface GitHubAppRepo {
  full_name: string
  name: string
  owner_login: string
  private: boolean
  default_branch: string
  clone_url: string
}

// GitHubAppBranch mirrors gitHubAppBranchResource
// (internal/api/github_app_repos.go).
export interface GitHubAppBranch {
  name: string
  commit_sha: string
}

// GitHubAppUseRepoAsSourceRequest mirrors useGitHubRepoAsSourceRequest
// (internal/api/github_app_use_as_source.go).
export interface GitHubAppUseRepoAsSourceRequest {
  app_name: string
  branch?: string
  build_type?: 'dockerfile' | 'railpack' | 'static'
  build_path?: string
}

// GitHubAppUseRepoAsSourceResponse mirrors
// useGitHubRepoAsSourceResponse (internal/api/github_app_use_as_source.go):
// the same GitSourceResource shape PUT .../git-source itself returns,
// extended with whether the push webhook was actually auto-registered.
// webhook_registered is false, with webhook_error explaining why, when
// the connected installation's permissions don't allow registering a
// webhook yet (an installation that predates the App requesting
// "Repository hooks: write") -- see that handler's own doc comment for
// why this degrades instead of failing the whole request.
export interface GitHubAppUseRepoAsSourceResponse extends GitSourceResource {
  webhook_registered: boolean
  webhook_error?: string
}

// GitHubAppManifestPreview mirrors githubAppManifestPreviewResource
// (internal/api/github_app_register.go): every field the manifest form
// is about to send GitHub, shown before the browser navigates away.
export interface GitHubAppManifestPreview {
  instance_url: string
  app_name: string
  homepage_url: string
  callback_url: string
  setup_url: string
  webhook_url: string
  webhook_active: boolean
  permissions: Record<string, string>
  events: string[]
  public: boolean
  request_oauth_on_install: boolean
}
