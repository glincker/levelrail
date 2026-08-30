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
  build_path?: string
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
  has_token: boolean
  webhook_url: string
  webhook_secret?: string
  created_at: string
  updated_at: string
}

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
  token?: string
  additional_services?: Record<string, GitSourceBuild>
  services?: Record<string, GitSourceService>
}
