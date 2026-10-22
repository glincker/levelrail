import { describe, expect, it, vi } from 'vitest'
import { prefetchNamespaces } from './prefetch'

describe('prefetchNamespaces', () => {
  it('loads only unloaded namespaces, in batches, one batch per idle slot', async () => {
    const loaded = new Set(['common'])
    const load = vi.fn((batch: string[]) => {
      batch.forEach((n) => loaded.add(n))
      return Promise.resolve()
    })
    const queue: Array<() => void> = []
    prefetchNamespaces({
      namespaces: ['common', 'a', 'b', 'c'],
      isLoaded: (ns) => loaded.has(ns),
      load,
      schedule: (run) => queue.push(run),
      batchSize: 2,
    })
    expect(load).not.toHaveBeenCalled()
    queue.shift()?.()
    await Promise.resolve()
    await Promise.resolve()
    queue.shift()?.()
    await Promise.resolve()
    await Promise.resolve()
    expect(load.mock.calls).toEqual([[['a', 'b']], [['c']]])
  })

  it('keeps going when a batch fails to load', async () => {
    const load = vi
      .fn<(batch: string[]) => Promise<unknown>>()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue(undefined)
    const queue: Array<() => void> = []
    prefetchNamespaces({
      namespaces: ['a', 'b'],
      isLoaded: () => false,
      load,
      schedule: (run) => queue.push(run),
      batchSize: 1,
    })
    queue.shift()?.()
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
    queue.shift()?.()
    expect(load).toHaveBeenCalledTimes(2)
  })
})
