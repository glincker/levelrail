// Fetchers for the import front door: POST /api/v1/imports/plan
// (internal/api/imports.go) returns a plan preview and creates nothing;
// deploying a plan reuses the existing create, build and compose
// endpoints, never a second deploy path.

import { ApiError, readErrorMessage } from '../lib/apiError'
import { createApp } from './apps'
import { deployCompose } from './compose'
import { triggerBuild } from './builds'

export type ImportSource =
  'repo' | 'docker_run' | 'image' | 'compose' | 'dockerfile'

export type ImportDeployKind = 'app' | 'build' | 'compose' | 'none'

export interface ImportEnvVar {
  key: string
  value?: string
  required: boolean
  has_default: boolean
  secret: boolean
  source?: string
}

export interface ImportPort {
  host?: number
  container: number
}

export interface ImportVolume {
  name?: string
  host_path?: string
  container_path: string
  read_only?: boolean
  needs_approval?: boolean
}

export interface ImportWarning {
  code: string
  message: string
}

export interface ImportService {
  name: string
  image?: string
  build: string
  build_reason?: string
  port?: number
  ports?: ImportPort[]
  command?: string[]
  health_path?: string
  env?: ImportEnvVar[]
  volumes?: ImportVolume[]
  restart?: string
}

export interface ImportPlan {
  source: ImportSource
  suggested_name: string
  deploy: ImportDeployKind
  repo_url?: string
  ref?: string
  compose_yaml?: string
  services: ImportService[]
  domain_suggestion?: string
  warnings: ImportWarning[]
  missing_required_env: string[]
}

export interface ImportPlanRequest {
  text: string
  kind?: ImportSource
  ref?: string
  name?: string
  port?: number
  env?: Record<string, string>
}

export async function planImport(req: ImportPlanRequest): Promise<ImportPlan> {
  const res = await fetch('/api/v1/imports/plan', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `plan import failed: ${res.status}`),
    )
  }
  return (await res.json()) as ImportPlan
}

export interface ImportDeployResult {
  appName: string
  /** Set for a git build: the deploy attempt whose live log to open. */
  deployId?: string
}

const PENDING_BUILD_TAG = ':pending'

function splitEnv(
  service: ImportService,
  values: Record<string, string>,
): {
  env: Record<string, string>
  secrets: Record<string, string>
} {
  const env: Record<string, string> = {}
  const secrets: Record<string, string> = {}
  const known = new Set<string>()
  for (const e of service.env ?? []) {
    known.add(e.key)
    const v = values[e.key] ?? e.value ?? ''
    if (v === '') continue
    if (e.secret) secrets[e.key] = v
    else env[e.key] = v
  }
  for (const [k, v] of Object.entries(values)) {
    if (!known.has(k) && v !== '') env[k] = v
  }
  return { env, secrets }
}

/**
 * Creates and deploys a plan through the existing endpoints: POST /apps
 * for an image, POST /apps then /builds for a repo, POST /apps/{name}/compose
 * for a compose plan (re-planned server side so env values land in the file).
 */
export async function deployImportPlan(
  plan: ImportPlan,
  request: ImportPlanRequest,
  values: Record<string, string>,
): Promise<ImportDeployResult> {
  const name = request.name || plan.suggested_name
  if (plan.deploy === 'compose') {
    const final = await planImport({ ...request, name, env: values })
    const res = await deployCompose(name, final.compose_yaml ?? '')
    return { appName: res.services[0]?.name ?? name }
  }
  const service = plan.services[0]
  if (!service || (plan.deploy !== 'app' && plan.deploy !== 'build')) {
    throw new Error('This input cannot be deployed on its own.')
  }
  const { env, secrets } = splitEnv(service, values)
  const secretKeys = Object.keys(secrets)
  await createApp({
    name,
    image:
      plan.deploy === 'build'
        ? `${name}${PENDING_BUILD_TAG}`
        : (service.image ?? ''),
    port: request.port ?? service.port ?? 0,
    ...(service.health_path
      ? { health: { readiness: { path: service.health_path } } }
      : {}),
    ...(Object.keys(env).length > 0 ? { env } : {}),
    ...(secretKeys.length > 0 ? { secret_env: secretKeys, secrets } : {}),
  })
  if (plan.deploy === 'app') return { appName: name }
  const built = await triggerBuild(name, {
    repoUrl: plan.repo_url ?? '',
    ref: plan.ref ?? 'main',
    buildType: service.build,
  })
  return { appName: name, deployId: built.id }
}
