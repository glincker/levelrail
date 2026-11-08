import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/apiError'
import { jobErrorMessage } from '../lib/jobAccess'
import { setScheduledTaskEnabled } from './scheduledTasks'

function reply(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('setScheduledTaskEnabled', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('posts to the enable and disable actions, not the edit PUT', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(reply({ id: 'j1' }, 200)))
    vi.stubGlobal('fetch', fetchMock)
    await setScheduledTaskEnabled('web', 'j1', true)
    await setScheduledTaskEnabled('web', 'j1', false)
    expect(fetchMock.mock.calls.map((c) => c as unknown[])).toEqual([
      ['/api/v1/apps/web/jobs/j1/enable', { method: 'POST' }],
      ['/api/v1/apps/web/jobs/j1/disable', { method: 'POST' }],
    ])
  })

  it('shows the access rule when the server refuses with 403', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(reply({ error: 'needs root' }, 403))),
    )
    const err = await setScheduledTaskEnabled('web', 'j1', true).catch(
      (e: unknown) => e,
    )
    expect(err).toBeInstanceOf(ApiError)
    expect(jobErrorMessage(err as Error, 'rule')).toBe('rule')
  })
})
