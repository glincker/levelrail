import { describe, expect, it, vi } from 'vitest'
import {
  DEFAULT_DEPLOY_OPTIONS,
  applyDeployOptions,
  failedOutcomes,
  jobError,
  newJob,
  previewSupport,
  setJobPreset,
  type ApplyDeps,
} from './deployOptions'

function deps(over: Partial<ApplyDeps> = {}): ApplyDeps {
  return {
    setPreviews: vi.fn().mockResolvedValue({}),
    ensureWebhook: vi.fn().mockResolvedValue(undefined),
    releaseOnly: vi.fn().mockResolvedValue(undefined),
    createJob: vi.fn().mockResolvedValue({}),
    ...over,
  }
}

describe('options state', () => {
  it('defaults to deploy on push, previews off, no jobs', () => {
    expect(DEFAULT_DEPLOY_OPTIONS).toEqual({
      deployOnPush: true,
      previews: false,
      healthPath: '',
      jobs: [],
    })
  })

  it('marks GitHub stable, other forges beta, unknown hosts none', () => {
    expect(previewSupport('https://github.com/a/b')).toBe('stable')
    expect(previewSupport('https://gitlab.com/a/b')).toBe('beta')
    expect(previewSupport('https://bitbucket.org/a/b')).toBe('beta')
    expect(previewSupport('https://git.example.com/a/b')).toBe('none')
    expect(previewSupport('not a url')).toBe('none')
  })

  it('maps presets to cron and validates jobs', () => {
    const j = setJobPreset(newJob(1), 'every5')
    expect(j.cron).toBe('*/5 * * * *')
    expect(jobError({ ...j, path: '/tick' })).toBeNull()
    expect(jobError({ ...j, path: 'tick' })).toBe('path')
    expect(jobError({ ...j, path: '/tick', cron: 'nope' })).toBe('cron')
    expect(setJobPreset(j, 'custom').cron).toBe('*/5 * * * *')
  })
})

describe('applyDeployOptions', () => {
  it('verifies the webhook and creates http jobs on success', async () => {
    const d = deps()
    const job = { ...setJobPreset(newJob(1), 'daily'), path: '/api/cron/x' }
    const out = await applyDeployOptions(
      'shop',
      { ...DEFAULT_DEPLOY_OPTIONS, previews: true, jobs: [job] },
      d,
    )
    expect(out.every((o) => o.ok)).toBe(true)
    expect(d.ensureWebhook).toHaveBeenCalledWith('shop')
    expect(d.setPreviews).toHaveBeenCalledWith('shop')
    expect(d.createJob).toHaveBeenCalledWith(
      'shop',
      expect.objectContaining({
        mode: 'http',
        schedule: '0 3 * * *',
        http: { method: 'GET', path: '/api/cron/x' },
      }),
    )
  })

  it('uses release-only mode when deploy on push is off', async () => {
    const d = deps()
    await applyDeployOptions(
      'shop',
      { ...DEFAULT_DEPLOY_OPTIONS, deployOnPush: false },
      d,
    )
    expect(d.releaseOnly).toHaveBeenCalledWith('shop')
    expect(d.ensureWebhook).not.toHaveBeenCalled()
  })

  it('reports each failure and keeps going', async () => {
    const d = deps({
      setPreviews: vi.fn().mockRejectedValue(new Error('no git source')),
      createJob: vi.fn().mockRejectedValue(new Error('bad cron')),
    })
    const job = { ...newJob(1), path: '/a' }
    const out = await applyDeployOptions(
      'shop',
      { ...DEFAULT_DEPLOY_OPTIONS, previews: true, jobs: [job] },
      d,
    )
    const failed = failedOutcomes(out)
    expect(failed.map((o) => o.key)).toEqual(['previews', 'job'])
    expect(failed.map((o) => o.reason)).toEqual(['no git source', 'bad cron'])
    expect(failed[1]?.subject).toBe('/a')
  })

  it('skips empty job rows and flags invalid ones without calling the api', async () => {
    const d = deps()
    const out = await applyDeployOptions(
      'shop',
      {
        ...DEFAULT_DEPLOY_OPTIONS,
        jobs: [newJob(1), { ...newJob(2), path: 'bad' }],
      },
      d,
    )
    expect(d.createJob).not.toHaveBeenCalled()
    expect(failedOutcomes(out)).toHaveLength(1)
  })
})
