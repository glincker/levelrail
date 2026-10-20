import { ApiError } from './apiError'
import { createApp } from '../queries/apps'
import { createAppConnection } from '../queries/appConnections'
import { createDatabase, fetchDatabase } from '../queries/databases'
import { triggerBuild } from '../queries/builds'
import { setGitSource } from '../queries/gitSources'
import { setGitBuild } from '../queries/gitBuildSettings'
import { connectGitHubRepoAsSource } from '../queries/githubApp'
import { connectRepoAsSource } from '../queries/gitConnections'
import type { ImportAnalysis } from '../queries/importAnalyze'
import type { NormalizedRepoOption } from './gitRepoOptions'
import type { GitSourceBuildType } from '../types/gitSource'
import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'

export const APP_NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/
export const PENDING_IMAGE_TAG = ':pending'
const MIB = 1024 * 1024
const DEFAULT_BRANCH = 'main'

/** Maps a picked repository onto the wizard's source value. */
export function sourceFromRepo(repo: NormalizedRepoOption): GitRepoSourceValue {
  return {
    provider: repo.providerRef.kind,
    repoUrl: repo.cloneUrl,
    branch: repo.defaultBranch,
    providerRef: repo.providerRef,
  }
}

export interface DatabaseEdit {
  enabled: boolean
  engine: string
  version: string
  name: string
  envVar: string
  field: string
}

export interface NewAppEdits {
  name: string
  port: number
  healthPath: string
  memoryMb: number
  buildType: GitSourceBuildType
  dockerfilePath: string
  baseDirectory: string
  domain: string
  /** Operator-typed env values; the repository's own values are never read. */
  env: Record<string, string>
  databases: DatabaseEdit[]
}

export type ChangeKind = 'app' | 'source' | 'database' | 'domain' | 'env'

export interface PlannedChange {
  kind: ChangeKind
  /** Stable id, also the i18n key suffix for the row title. */
  id: string
  label: string
  detail: string
}

export type ApplyStep = 'database' | 'app' | 'connection' | 'source' | 'build'

export class ApplyStepError extends Error {
  readonly step: ApplyStep
  readonly appCreated: boolean
  constructor(step: ApplyStep, cause: unknown, appCreated: boolean) {
    super(cause instanceof Error ? cause.message : 'Unknown error')
    this.name = 'ApplyStepError'
    this.step = step
    this.appCreated = appCreated
  }
}

function asBuildType(raw: string): GitSourceBuildType {
  return raw === 'dockerfile' || raw === 'static' ? raw : 'railpack'
}

export function editsFromAnalysis(a: ImportAnalysis): NewAppEdits {
  return {
    name: a.name.value,
    port: a.port.value,
    healthPath: a.health.value,
    memoryMb: Math.max(1, Math.round(a.memory.bytes / MIB)),
    buildType: asBuildType(a.build.type),
    dockerfilePath: a.build.dockerfile_path ?? '',
    baseDirectory: a.build.base_directory ?? '',
    domain: a.domain.value,
    env: {},
    databases: a.databases.map((d) => ({
      enabled: true,
      engine: d.engine,
      version: d.version,
      name: d.suggested_name,
      envVar: d.env_var,
      field: d.field || 'url',
    })),
  }
}

/** Env keys the operator must still fill in: required and not DB-provided. */
export function missingRequiredEnv(
  a: ImportAnalysis,
  e: NewAppEdits,
): string[] {
  const provided = new Set(
    e.databases.filter((d) => d.enabled).map((d) => d.envVar),
  )
  return a.env
    .filter(
      (k) =>
        k.required &&
        !provided.has(k.key) &&
        (e.env[k.key] ?? '').trim() === '',
    )
    .map((k) => k.key)
}

export function validateEdits(e: NewAppEdits): string[] {
  const problems: string[] = []
  if (!APP_NAME_PATTERN.test(e.name)) problems.push('name')
  if (!(e.port > 0 && e.port < 65536)) problems.push('port')
  if (e.buildType === 'dockerfile' && e.dockerfilePath.trim() === '') {
    problems.push('dockerfilePath')
  }
  for (const d of e.databases) {
    if (d.enabled && !APP_NAME_PATTERN.test(d.name)) problems.push('database')
  }
  return problems
}

/** A diff-like preview: everything the Deploy button will create. */
export function plannedChanges(
  a: ImportAnalysis,
  e: NewAppEdits,
): PlannedChange[] {
  const out: PlannedChange[] = [
    {
      kind: 'app',
      id: 'app',
      label: e.name,
      detail: `port ${String(e.port)}, ${String(e.memoryMb)} MiB, health ${e.healthPath || '/'}`,
    },
    {
      kind: 'source',
      id: 'source',
      label: a.repo_url,
      detail: [
        a.branch,
        e.buildType,
        e.dockerfilePath,
        e.baseDirectory ? `context ${e.baseDirectory}` : '',
      ]
        .filter((p) => p !== '')
        .join(', '),
    },
  ]
  for (const d of e.databases) {
    if (!d.enabled) continue
    out.push({
      kind: 'database',
      id: `database:${d.name}`,
      label: d.name,
      detail: `${d.engine} ${d.version}, injected as ${d.envVar}`,
    })
  }
  if (e.domain.trim() !== '') {
    out.push({ kind: 'domain', id: 'domain', label: e.domain, detail: '' })
  }
  const keys = a.env.filter(
    (k) => k.provided_by === undefined || k.provided_by === '',
  )
  for (const k of keys) {
    const set = (e.env[k.key] ?? '') !== ''
    if (!set && !k.required) continue
    out.push({
      kind: 'env',
      id: `env:${k.key}`,
      label: k.key,
      detail: set ? (k.secret ? 'secret' : 'value') : 'required',
    })
  }
  return out
}

async function connectSource(
  appName: string,
  source: GitRepoSourceValue,
  e: NewAppEdits,
): Promise<void> {
  const base = {
    app_name: appName,
    branch: source.branch,
    build_type: e.buildType,
    build_path: e.dockerfilePath || undefined,
  }
  const ref = source.providerRef
  switch (ref?.kind) {
    case 'github':
      await connectGitHubRepoAsSource(ref.owner, ref.repo, base)
      break
    case 'gitlab':
    case 'bitbucket':
    case 'gitea':
      await connectRepoAsSource(ref.connectionId, { ...base, repo: ref.repo })
      break
    default:
      await setGitSource(appName, {
        repo_url: source.repoUrl,
        branch: source.branch,
        build_type: e.buildType,
        build_path: e.dockerfilePath || undefined,
        base_directory: e.baseDirectory || undefined,
        token: source.token,
      })
      return
  }
  if (e.baseDirectory !== '') {
    await setGitBuild(appName, {
      build_type: e.buildType,
      build_path: e.dockerfilePath,
      base_directory: e.baseDirectory,
    })
  }
}

async function ensureDatabase(d: DatabaseEdit): Promise<void> {
  try {
    await fetchDatabase(d.name)
    return
  } catch (err) {
    if (!(err instanceof ApiError) || err.status !== 404) throw err
  }
  await createDatabase({ name: d.name, engine: d.engine, version: d.version })
}

export interface ApplyResult {
  appName: string
  deployId: string
}

/**
 * Creates the databases, the app, its connections and repository link, then
 * starts the first build: the same endpoints the CLI's import --apply uses.
 * Each failure names its step; nothing is rolled back. resume skips app
 * creation for a retry after the app record already exists.
 */
export async function applyNewApp(
  a: ImportAnalysis,
  source: GitRepoSourceValue,
  e: NewAppEdits,
  resume = false,
): Promise<ApplyResult> {
  const dbs = e.databases.filter((d) => d.enabled)
  let step: ApplyStep = 'database'
  let appCreated = false
  try {
    for (const d of dbs) await ensureDatabase(d)
    step = 'app'
    const secretKeys = a.env.filter((k) => k.secret).map((k) => k.key)
    const env: Record<string, string> = {}
    const secrets: Record<string, string> = {}
    for (const [k, v] of Object.entries(e.env)) {
      if (v.trim() === '') continue
      if (secretKeys.includes(k)) secrets[k] = v
      else env[k] = v
    }
    if (!resume) {
      await createApp({
        name: e.name,
        image: `${e.name}${PENDING_IMAGE_TAG}`,
        port: e.port,
        ...(e.domain.trim() !== '' ? { domains: [e.domain.trim()] } : {}),
        ...(e.healthPath
          ? { health: { readiness: { path: e.healthPath } } }
          : {}),
        ...(Object.keys(env).length > 0 ? { env } : {}),
        ...(Object.keys(secrets).length > 0
          ? { secret_env: Object.keys(secrets), secrets }
          : {}),
        resources: { memory_bytes: e.memoryMb * MIB },
      })
    }
    appCreated = true
    step = 'connection'
    for (const d of dbs) {
      await createAppConnection(e.name, {
        database: d.name,
        field: d.field,
        envVar: d.envVar,
      })
    }
    step = 'source'
    await connectSource(e.name, source, e)
    step = 'build'
    const built = await triggerBuild(e.name, {
      repoUrl: source.repoUrl,
      ref: source.branch || DEFAULT_BRANCH,
      buildType: e.buildType,
      buildPath: e.dockerfilePath || undefined,
      baseDirectory: e.baseDirectory || undefined,
      detectedFramework: a.stack.framework,
    })
    if (!built.id)
      throw new Error('the build did not return a deploy attempt id')
    return { appName: e.name, deployId: built.id }
  } catch (err) {
    throw new ApplyStepError(step, err, appCreated)
  }
}
