import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchAppStatus } from './appStatus'

afterEach(() => vi.unstubAllGlobals())

describe('fetchAppStatus', () => {
  it('returns the server outcome fields', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            conditions: [],
            outcome: 'failed',
            outcome_reason: 'crash looping',
            rollout_crash_loop_restarts: 4,
          }),
        ),
      ),
    )
    const s = await fetchAppStatus('web')
    expect(s.outcome).toBe('failed')
    expect(s.rollout_crash_loop_restarts).toBe(4)
  })

  it('falls back to no outcome when an older server answers 404', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('not found', { status: 404 })),
    )
    expect(await fetchAppStatus('web')).toEqual({ conditions: [] })
  })
})
