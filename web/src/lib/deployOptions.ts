import { parseCron } from './cronNext'
import {
  fetchGitSource,
  registerGitSourceWebhook,
  setGitSource,
} from '../queries/gitSources'
import { setPreviewEnabled } from '../queries/previewEnvironments'
import { createScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTaskRequest } from '../types/scheduledTasks'

export const CRON_PRESETS = [
  { id: 'every5', cron: '*/5 * * * *' },
  { id: 'hourly', cron: '0 * * * *' },
  { id: 'daily', cron: '0 3 * * *' },
] as const

export type JobPreset = (typeof CRON_PRESETS)[number]['id'] | 'custom'

export interface JobDraft {
  id: number
  path: string
  preset: JobPreset
  cron: string
}

export interface DeployOptionsValue {
  deployOnPush: boolean
  previews: boolean
  healthPath: string
  jobs: JobDraft[]
}

export type PreviewSupport = 'stable' | 'beta' | 'none'

export const DEFAULT_DEPLOY_OPTIONS: DeployOptionsValue = {
  deployOnPush: true,
  previews: false,
  healthPath: '',
  jobs: [],
}

/** GitHub previews are stable; the other hosts are beta; unknown hosts cannot. */
export function previewSupport(repoUrl: string): PreviewSupport {
  let host: string
  try {
    host = new URL(repoUrl).hostname.toLowerCase().replace(/^www\./, '')
  } catch {
    return 'none'
  }
  if (host === 'github.com') return 'stable'
  if (
    host === 'bitbucket.org' ||
    host.startsWith('gitlab.') ||
    host.startsWith('gitea.') ||
    host.startsWith('forgejo.')
  )
    return 'beta'
  return 'none'
}

export function newJob(id: number): JobDraft {
  return { id, path: '', preset: 'hourly', cron: '0 * * * *' }
}

export function setJobPreset(job: JobDraft, preset: JobPreset): JobDraft {
  const found = CRON_PRESETS.find((p) => p.id === preset)
  return { ...job, preset, cron: found ? found.cron : job.cron }
}

export type JobError = 'path' | 'cron' | null

export function jobError(job: JobDraft): JobError {
  if (!job.path.trim().startsWith('/')) return 'path'
  if (parseCron(job.cron) === null) return 'cron'
  return null
}

export type OptionKey = 'deployOnPush' | 'previews' | 'job'

export interface OptionOutcome {
  key: OptionKey
  ok: boolean
  /** The job path for a job outcome, else empty. */
  subject: string
  reason: string
}

export interface ApplyDeps {
  setPreviews: (app: string) => Promise<unknown>
  ensureWebhook: (app: string) => Promise<void>
  releaseOnly: (app: string) => Promise<void>
  createJob: (app: string, req: ScheduledTaskRequest) => Promise<unknown>
}

export const realDeps: ApplyDeps = {
  setPreviews: (app) => setPreviewEnabled(app, true),
  ensureWebhook: async (app) => {
    let src = await fetchGitSource(app)
    if (src.webhook_registration?.state === 'failed') {
      src = await registerGitSourceWebhook(app)
    }
    if (src.webhook_registration?.state === 'failed') {
      throw new Error(src.webhook_registration.error ?? 'webhook failed')
    }
    if (src.webhook_url_warning) throw new Error(src.webhook_url_warning)
  },
  releaseOnly: async (app) => {
    const src = await fetchGitSource(app)
    await setGitSource(app, {
      repo_url: src.repo_url,
      branch: src.branch,
      build_type: src.build_type,
      build_path: src.build_path,
      base_directory: src.base_directory,
      build_command: src.build_command,
      publish_dir: src.publish_dir,
      trigger_mode: 'release',
    })
  },
  createJob: createScheduledTask,
}

function reasonOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function jobSlug(path: string): string {
  const slug = path
    .replace(/[^a-z0-9]+/gi, '-')
    .replace(/^-+|-+$/g, '')
    .toLowerCase()
  return `import-${slug || 'job'}`
}

/**
 * Applies the chosen options through the per-app APIs after the app exists.
 * Every step is attempted and reported and none throws, so a failure never
 * hides that the app itself was created.
 */
export async function applyDeployOptions(
  app: string,
  opts: DeployOptionsValue,
  deps: ApplyDeps = realDeps,
): Promise<OptionOutcome[]> {
  const out: OptionOutcome[] = []
  const run = async (
    key: OptionKey,
    subject: string,
    fn: () => Promise<unknown>,
  ) => {
    try {
      await fn()
      out.push({ key, ok: true, subject, reason: '' })
    } catch (err) {
      out.push({ key, ok: false, subject, reason: reasonOf(err) })
    }
  }
  await run('deployOnPush', '', () =>
    opts.deployOnPush ? deps.ensureWebhook(app) : deps.releaseOnly(app),
  )
  if (opts.previews) await run('previews', '', () => deps.setPreviews(app))
  for (const j of opts.jobs) {
    const path = j.path.trim()
    if (path === '') continue
    const bad = jobError(j)
    if (bad) {
      out.push({ key: 'job', ok: false, subject: path, reason: bad })
      continue
    }
    await run('job', path, () =>
      deps.createJob(app, {
        name: jobSlug(path),
        mode: 'http',
        command: [],
        http: { method: 'GET', path },
        schedule: j.cron.trim(),
        timezone: 'UTC',
        enabled: true,
      }),
    )
  }
  return out
}

export function failedOutcomes(list: OptionOutcome[]): OptionOutcome[] {
  return list.filter((o) => !o.ok)
}
