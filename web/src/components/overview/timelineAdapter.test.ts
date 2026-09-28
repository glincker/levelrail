import { afterEach, describe, expect, it, vi } from 'vitest'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { ReconcileCondition } from '../../types/deploy'
import { fetchTimeline } from '../../queries/appTimeline'
import { fallbackTimeline } from './timelineAdapter'
import { toTimelineItems } from './timelineItems'

const attempt = (over: Partial<DeployAttempt>): DeployAttempt => ({
  id: 'a1',
  service_name: 'web',
  image: 'reg.io/web:v2@sha256:abc',
  status: 'succeeded',
  started_at: '2026-01-02T00:00:00Z',
  source: 'image',
  ...over,
})
const condition: ReconcileCondition = {
  Type: 'Ready',
  Status: 'True',
  Reason: 'Deployed',
  Message: '',
  LastTransitionTime: '2026-01-03T00:00:00Z',
}
const app = { env_dirty: false, suspended: false }

describe('fallbackTimeline', () => {
  it('maps attempts to deploy and rollback entries, newest first', () => {
    const out = fallbackTimeline(
      [
        attempt({
          id: 'old',
          started_at: '2026-01-01T00:00:00Z',
          status: 'failed',
          error: 'boom',
        }),
        attempt({
          id: 'rb',
          source: 'auto_rollback',
          started_at: '2026-01-02T00:00:00Z',
        }),
      ],
      app,
      [condition],
    )
    expect(out.map((e) => e.id)).toEqual(['attempt-rb', 'attempt-old'])
    expect(out[0]).toMatchObject({
      kind: 'rollback',
      title: 'Rolled back to v2',
      actor: 'auto rollback',
    })
    expect(out[1]).toMatchObject({
      kind: 'deploy',
      status: 'failed',
      detail: 'boom',
      ref: { type: 'deploy_attempt', id: 'old' },
    })
  })

  it('titles git deploys by short sha', () => {
    const [e] = fallbackTimeline(
      [attempt({ source: 'webhook', commit_sha: 'abcdef1234567' })],
      app,
      [],
    )
    expect(e?.title).toBe('Deploy abcdef1')
    expect(e?.actor).toBe('git push')
  })

  it('infers pending env change and stop from the app payload', () => {
    const out = fallbackTimeline([], { env_dirty: true, suspended: true }, [
      condition,
    ])
    expect(out.map((e) => e.kind).sort()).toEqual(['env_change', 'suspend'])
    expect(out.find((e) => e.kind === 'env_change')?.status).toBe('pending')
  })

  it('limits to the requested count', () => {
    const many = Array.from({ length: 12 }, (_, i) =>
      attempt({
        id: `a${i}`,
        started_at: `2026-01-${String(i + 1).padStart(2, '0')}T00:00:00Z`,
      }),
    )
    expect(fallbackTimeline(many, app, [], 8)).toHaveLength(8)
  })
})

describe('toTimelineItems', () => {
  it('makes only deploy entries clickable', () => {
    const opened: string[] = []
    const items = toTimelineItems(
      fallbackTimeline(
        [attempt({ id: 'x' })],
        { env_dirty: true, suspended: false },
        [condition],
      ),
      (id) => opened.push(id),
    )
    const deploy = items.find((i) => i.id === 'attempt-x')
    const env = items.find((i) => i.id === 'inferred-env-pending')
    deploy?.onClick?.()
    expect(opened).toEqual(['x'])
    expect(env?.onClick).toBeUndefined()
  })
})

describe('fetchTimeline', () => {
  afterEach(() => vi.restoreAllMocks())

  it('resolves to null on 404 so the page can fall back', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 404,
    } as Response)
    await expect(fetchTimeline('web')).resolves.toBeNull()
  })

  it('returns items when the endpoint exists', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ items: [{ id: '1' }] }),
    } as unknown as Response)
    await expect(fetchTimeline('web')).resolves.toEqual([{ id: '1' }])
  })
})
