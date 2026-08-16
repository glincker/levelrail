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

export interface GitSourceResource {
  service_name: string
  repo_url: string
  branch: string
  build_type: GitSourceBuildType
  build_path?: string
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
}
