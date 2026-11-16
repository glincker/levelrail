import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MAIN_SENTINEL_ATTR, useMainScrolled } from './useMainScrolled'
import { MAIN_CONTENT_ID } from './SkipLink'

let notify: (intersecting: boolean) => void = () => {}

describe('useMainScrolled', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(cb: IntersectionObserverCallback) {
          notify = (isIntersecting) =>
            cb(
              [{ isIntersecting } as IntersectionObserverEntry],
              this as unknown as IntersectionObserver,
            )
        }
        observe() {}
        disconnect() {}
      },
    )
    document.body.innerHTML = `<main id="${MAIN_CONTENT_ID}"><div ${MAIN_SENTINEL_ATTR}></div></main>`
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('flips when the sentinel leaves and re-enters the viewport', () => {
    const { result } = renderHook(() => useMainScrolled())
    expect(result.current).toBe(false)
    act(() => notify(false))
    expect(result.current).toBe(true)
    act(() => notify(true))
    expect(result.current).toBe(false)
  })
})
