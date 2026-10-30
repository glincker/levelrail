// Wire type for the git source resource, GET/PUT/DELETE
// /api/v1/apps/{name}/git-source (internal/api/git_sources.go's
// gitSourceResource). Field names mirror the JSON wire shape directly,
// the same snake_case-matches-wire-shape convention appDetail.ts/
// databaseDetail.ts already document.
//
// webhook_secret: response-only, and only ever populated the one time
// PUT creates a new git source (never on GET, never on an update PUT):
// see that handler's own doc comment for why this is a deliberate,
// one-time exception to this codebase's "never decrypt a secret back
// into a response" rule. There is no request-body counterpart field for
// it; it is generated server-side, never client-supplied.

export type GitSourceBuildType = 'dockerfile' | 'railpack' | 'static'

// GitSourceTriggerMode mirrors internal/spec's TriggerModePush/
// TriggerModeRelease: 'push' (default) deploys on every push to branch,
// 'release' deploys only on a tag ref push, or (GitHub only) a
// "release" webhook event with action "published".
export type GitSourceTriggerMode = 'push' | 'release'

export interface GitSourceResolvedBuild {
  context_dir: string
  dockerfile_path?: string
  summary: string
}

export interface GitBuildSuggestion {
  build_type: GitSourceBuildType
  dockerfile_path?: string
  base_directory?: string
  reason: string
  reason_code: string
  recommended: boolean
}

// Mirrors internal/api's detectGitSourceResponse.
export interface WorkspacePackage {
  name: string
  dir: string
  tool: string
  dockerfile?: string
  depends_on: string[]
  watch_paths: string[]
}

export interface GitSourceAffected {
  app_deploys: boolean
  reason?: string
  packages: { name: string; dir: string; affected: boolean }[]
}

export interface GitBuildDetection {
  branch: string
  looks_like_monorepo: boolean
  root_has_app: boolean
  tools: string[]
  dockerfiles: string[]
  compose_files: string[]
  suggestions: GitBuildSuggestion[]
  truncated: boolean
  needs_build_settings: boolean
  packages: WorkspacePackage[]
  static_build?: StaticBuildSuggestion
}

// StaticBuildSuggestion prefills a static build from the repository's framework preset.
export interface StaticBuildSuggestion {
  preset_id: string
  preset_name: string
  command: string
  publish_dir: string
}

export interface SetGitSourceBuildRequest {
  build_type: GitSourceBuildType
  build_path: string
  base_directory: string
  build_command?: string
  publish_dir?: string
}

export interface GitSourceBuild {
  build_type: GitSourceBuildType
  build_path?: string
}

// GitSourceServiceBuild mirrors internal/spec.Build's own wire shape,
// the subset this form actually collects (build.type/path/image): the
// same subset DeploySpecServiceInput (appGroup.ts) already collects for
// the manual/API deploy-spec form, since a persisted services: map is
// the identical shape, just saved on the git source instead of sent
// per-request.
export interface GitSourceServiceBuild {
  type: 'dockerfile' | 'railpack' | 'static' | 'image' | 'compose'
  path?: string
  image?: string
}

export interface GitSourceService {
  build: GitSourceServiceBuild
  port?: number
  domains?: string[]
}

export interface GitSourceResource {
  service_name: string
  repo_url: string
  branch: string
  build_type: GitSourceBuildType
  // build_path is the Dockerfile (or static output directory), relative to
  // the repository root.
  build_path?: string
  // base_directory is the build context relative to the repository root;
  // empty or absent means the root.
  base_directory?: string
  // build_command and publish_dir build a static site before serving it.
  build_command?: string
  publish_dir?: string
  resolved_build?: GitSourceResolvedBuild
  // additional_services fans one push out to sibling services under the
  // same app group (apps_group.go), keyed by the sibling's own service
  // name. Mutually exclusive with services, see that field's own doc
  // comment.
  additional_services?: Record<string, GitSourceBuild>
  // services is an app.yaml-style services: map (store.GitSource.Services's
  // own doc comment): when set, a push fans out through the same
  // deploy.Pipeline.DeploySpec logic POST .../deploy-spec uses, instead
  // of additional_services's own flat rebuild list.
  services?: Record<string, GitSourceService>
  // trigger_mode mirrors store.GitSource.TriggerMode: always populated by
  // the server (empty-string-safe zero value normalized to 'push'), see
  // GitSourceTriggerMode's own doc comment.
  trigger_mode: GitSourceTriggerMode
  has_token: boolean
  webhook_url: string
  // webhook_url_warning is set when webhook_url cannot be reached by a git
  // host: a localhost or private address, or no public address at all.
  webhook_url_warning?: 'private_address' | 'no_public_url'
  webhook_url_source?: string
  // webhook_registration is the outcome of registering the webhook on the provider.
  webhook_registration?: {
    state: 'registered' | 'failed'
    error?: string
    changed_at?: string
  }
  // poll is the missed-webhook poller's view of this source.
  poll?: {
    enabled: boolean
    last_polled_at?: string
    last_webhook_at?: string
    last_seen_head?: string
    error?: string
  }
  /** The provider connection the repository was picked from; absent for a pasted URL. */
  connection_id?: string
  connection?: {
    id: string
    provider?: 'gitlab' | 'bitbucket' | 'gitea'
    label?: string
    account_name?: string
    host?: string
    /** ok, needs_reauthorize, revoked, rate_limited, or needs_reconnect once the account was disconnected. */
    state: string
    detail?: string
  }
  webhook_secret?: string
  // preview_enabled mirrors store.GitSource.PreviewEnabled: read-only
  // here, set via PUT /api/v1/apps/{name}/preview-settings
  // (queries/previewEnvironments.ts's setPreviewEnabled), not this
  // resource's own PUT.
  preview_enabled: boolean
  // post_pr_comments mirrors store.GitSource.PostPRComments: same
  // read-only-here, set-via-preview-settings shape as preview_enabled
  // above (queries/previewEnvironments.ts's setPreviewPostPRComments).
  post_pr_comments: boolean
  // pr_comment_commands and pr_comment_commands_forks mirror the store
  // fields: /levelrail pull request comment commands, off by default.
  pr_comment_commands?: boolean
  pr_comment_commands_forks?: boolean
  // deploy_paths and deploy_paths_ignore filter which pushes deploy by the
  // files they changed; report_status posts deploy state back to the forge.
  // Set via PUT /api/v1/apps/{name}/git-source/deploy-settings.
  deploy_paths: string[]
  deploy_paths_ignore: string[]
  report_status: boolean
  wait_for_ci?: boolean
  ci_required_checks?: string[]
  created_at: string
  updated_at: string
}

// Mirrors gitSourceSummaryResource (internal/api/git_sources.go).
export interface GitSourceSummaryResource {
  service_name: string
  repo_url: string
  branch: string
}

export interface GitDeploySettings {
  deploy_paths: string[]
  deploy_paths_ignore: string[]
  report_status: boolean
  wait_for_ci?: boolean
  ci_required_checks?: string[]
}

export type SetGitDeploySettingsRequest = Partial<GitDeploySettings>

// SetGitSourceRequest is PUT /api/v1/apps/{name}/git-source's body.
// token is write-only, and only sent at all when the operator actually
// typed one: an empty/omitted token on an update leaves whatever was
// previously stored unchanged (setGitSourceRequest's own doc comment,
// internal/api/git_sources.go), it never clears it.
export interface SetGitSourceRequest {
  repo_url: string
  branch?: string
  build_type?: GitSourceBuildType
  build_path?: string
  base_directory?: string
  build_command?: string
  publish_dir?: string
  token?: string
  additional_services?: Record<string, GitSourceBuild>
  services?: Record<string, GitSourceService>
  trigger_mode?: GitSourceTriggerMode
}
