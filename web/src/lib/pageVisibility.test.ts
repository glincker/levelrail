import { afterEach, describe, expect, it, vi } from 'vitest'
import { setVisibleInterval } from './pageVisibility'

function setVisibility(v: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => v,
  })
}

describe('setVisibleInterval', () => {
  afterEach(() => {
    vi.useRealTimers()
    setVisibility('visible')
  })

  it('skips ticks while hidden and fires on return', () => {
    vi.useFakeTimers()
    const fn = vi.fn()
    const stop = setVisibleInterval(fn, 1000)
    vi.advanceTimersByTime(2000)
    expect(fn).toHaveBeenCalledTimes(2)
    setVisibility('hidden')
    vi.advanceTimersByTime(3000)
    expect(fn).toHaveBeenCalledTimes(2)
    setVisibility('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(fn).toHaveBeenCalledTimes(3)
    stop()
  })
})
