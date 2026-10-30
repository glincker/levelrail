import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useBoundedRecheck } from './useBoundedRecheck'

const delays = [10, 20, 40]
beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('useBoundedRecheck', () => {
  it('stops after the last attempt', async () => {
    const run = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() =>
      useBoundedRecheck({ active: true, done: false, run, delays }),
    )
    for (let i = 0; i < 6; i++)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(100)
      })
    expect(run).toHaveBeenCalledTimes(3)
    expect(result.current.exhausted).toBe(true)
  })
  it('stops on unmount', async () => {
    const run = vi.fn().mockResolvedValue(undefined)
    const { unmount } = renderHook(() =>
      useBoundedRecheck({ active: true, done: false, run, delays }),
    )
    unmount()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    expect(run).not.toHaveBeenCalled()
  })
  it('does not run when every record is live', async () => {
    const run = vi.fn().mockResolvedValue(undefined)
    renderHook(() =>
      useBoundedRecheck({ active: true, done: true, run, delays }),
    )
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    expect(run).not.toHaveBeenCalled()
  })
})
