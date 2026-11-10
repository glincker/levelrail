import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import {
  primeShellCache,
  resetShellCacheForTest,
  seedShellCache,
  shellCacheReady,
  shellSectionKeys,
  shellSeedTimeoutMs,
} from './shell'

afterEach(() => {
  vi.unstubAllGlobals()
  resetShellCacheForTest()
})

describe('seedShellCache', () => {
  it('seeds each returned section under the key its own query reads', () => {
    const qc = new QueryClient()
    const keys = shellSectionKeys()
    const n = seedShellCache(qc, {
      sections: {
        brand: { name: 'Acme' },
        projects: [{ id: 'p1' }],
        appearance: { value: { theme: 'dark' } },
        unknown_section: { x: 1 },
      },
    })
    expect(n).toBe(3)
    expect(qc.getQueryData(keys.brand)).toEqual({ name: 'Acme' })
    expect(qc.getQueryData(keys.projects)).toEqual([{ id: 'p1' }])
    expect(qc.getQueryData(keys.appearance)).toEqual({
      available: true,
      value: { theme: 'dark' },
    })
  })

  it('leaves omitted sections and already cached keys alone', () => {
    const qc = new QueryClient()
    const keys = shellSectionKeys()
    qc.setQueryData(keys.local_mode, { enabled: true, reset_supported: false })
    seedShellCache(qc, { sections: { local_mode: { enabled: false } } })
    expect(qc.getQueryData(keys.local_mode)).toEqual({
      enabled: true,
      reset_supported: false,
    })
    expect(qc.getQueryData(keys.system_status)).toBeUndefined()
  })
})

describe('primeShellCache', () => {
  it('fetches the aggregate once and swallows failures', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(new Response('', { status: 401 })),
    )
    vi.stubGlobal('fetch', fetchMock)
    const qc = new QueryClient()
    await primeShellCache(qc)
    await primeShellCache(qc)
    await shellCacheReady()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(qc.getQueryData(shellSectionKeys().brand)).toBeUndefined()
  })
})

describe('shellCacheReady', () => {
  it('stops waiting after the seed timeout when the aggregate hangs', async () => {
    vi.useFakeTimers()
    try {
      vi.stubGlobal(
        'fetch',
        vi.fn(() => new Promise<Response>(() => undefined)),
      )
      void primeShellCache(new QueryClient())
      let done = false
      const ready = shellCacheReady().then(() => {
        done = true
      })
      await vi.advanceTimersByTimeAsync(shellSeedTimeoutMs() + 1)
      await ready
      expect(done).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })

  it('reads the timeout from env with a default', () => {
    expect(shellSeedTimeoutMs({ VITE_SHELL_SEED_TIMEOUT_MS: '800' })).toBe(800)
    expect(shellSeedTimeoutMs({})).toBeGreaterThan(0)
  })
})
