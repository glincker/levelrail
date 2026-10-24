import { ApiError, readErrorMessage } from '../lib/apiError'

export type ImportApplyStatus =
  | 'created'
  | 'already_exists'
  | 'skipped_dependency'
  | 'failed'
  | 'would_create'

export interface ImportApplyPackage {
  package: string
  name: string
  directory: string
  port?: number
  env?: Record<string, string>
  secret_env_keys?: string[]
  project_id?: string
  environment_id?: string
  deploy?: boolean
}

export interface ImportApplyRequest {
  repo_url: string
  branch?: string
  base_directory?: string
  /** One-shot token for a private pasted repo; the server never echoes it. */
  token?: string
  packages: ImportApplyPackage[]
  dry_run?: boolean
  confirm?: boolean
}

export interface ImportApplyResult {
  package: string
  name: string
  directory: string
  status: ImportApplyStatus
  code?: string
  message?: string
  app_id?: string
  deploy_id?: string
  deploy_error?: string
}

export interface ImportApplyResponse {
  repo_url: string
  branch: string
  dry_run: boolean
  needs_confirmation: boolean
  confirm_above: number
  counts: Record<string, number>
  results: ImportApplyResult[]
}

export async function applyRepoImport(
  req: ImportApplyRequest,
): Promise<ImportApplyResponse> {
  const res = await fetch('/api/v1/imports/repo/apply', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `import failed: ${res.status}`),
    )
  }
  return (await res.json()) as ImportApplyResponse
}
