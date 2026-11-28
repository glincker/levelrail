import { renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MAIN_CONTENT_ID } from './SkipLink'
import { useResetMainScroll } from './useResetMainScroll'

describe('useResetMainScroll', () => {
  let main: HTMLElement
  const scrollIntoView = vi.fn()
  const focus = vi.fn()

  beforeEach(() => {
    main = document.createElement('main')
    main.id = MAIN_CONTENT_ID
    document.body.appendChild(main)
    scrollIntoView.mockClear()
    focus.mockClear()
    Element.prototype.scrollIntoView = scrollIntoView
    HTMLElement.prototype.focus = focus
  })
  afterEach(() => main.remove())

  it('resets a carried-over offset when the path changes, without scroll or focus calls', () => {
    main.scrollTop = 480
    const { rerender } = renderHook(({ p }) => useResetMainScroll(p, ''), {
      initialProps: { p: '/apps' },
    })
    expect(main.scrollTop).toBe(0)
    main.scrollTop = 300
    rerender({ p: '/databases/postgres/overview' })
    expect(main.scrollTop).toBe(0)
    expect(scrollIntoView).not.toHaveBeenCalled()
    expect(focus).not.toHaveBeenCalled()
  })

  it('leaves a hash deep link alone', () => {
    main.scrollTop = 300
    renderHook(() =>
      useResetMainScroll('/databases/postgres/overview', 'backups'),
    )
    expect(main.scrollTop).toBe(300)
  })
})
