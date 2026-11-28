import { ApiError } from '../lib/apiError'

export interface EnvApplyBody {
  expected_version?: number
  set?: Record<string, string>
  unset?: string[]
  secret_set?: Record<string, string>
  secret_delete?: string[]
  vault_set?: Record<string, { path: string; key: string }>
  vault_clear?: string[]
  overwrite_locked?: boolean
  dry_run?: boolean
}

export interface EnvApplyResult {
  changed: boolean
  env_version: number
  plan: Array<{ key: string; action: string }>
}

// A stale expected_version: nothing was written, the caller keeps its draft.
export class EnvVersionConflictError extends ApiError {
  readonly currentVersion: number
  readonly planKeys: string[]

  constructor(message: string, currentVersion: number, planKeys: string[]) {
    super(409, message)
    this.name = 'EnvVersionConflictError'
    this.currentVersion = currentVersion
    this.planKeys = planKeys
  }
}

// One transaction for plain, secret and vault changes (POST .../env/apply).
export async function applyEnv(
  app: string,
  body: EnvApplyBody,
): Promise<EnvApplyResult> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(app)}/env/apply`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    cache: 'no-store',
  })
  if (res.ok) return (await res.json()) as EnvApplyResult
  const err = (await res.json().catch(() => null)) as {
    error?: string
    code?: string
    current_version?: number
    plan_keys?: string[]
  } | null
  const message = err?.error ?? `request failed: ${String(res.status)}`
  if (res.status === 409 && err?.code === 'env_version_conflict') {
    throw new EnvVersionConflictError(
      message,
      err.current_version ?? 0,
      err.plan_keys ?? [],
    )
  }
  throw new ApiError(res.status, message)
}
