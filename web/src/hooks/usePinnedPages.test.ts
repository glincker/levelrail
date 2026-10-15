import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { usePinnedPages } from './usePinnedPages'

const PINNED_PAGES_KEY = 'levelrail:pinned-pages'

describe('usePinnedPages', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('pins and retrieves pages', () => {
    const { result } = renderHook(() => usePinnedPages())

    result.current.pin('/getting-started', 'Getting Started')

    const stored = JSON.parse(
      localStorage.getItem(PINNED_PAGES_KEY) || '[]',
    ) as Array<{ path: string; title: string }>
    expect(stored).toEqual([
      { path: '/getting-started', title: 'Getting Started' },
    ])
  })

  it('unpins a page', () => {
    const { result } = renderHook(() => usePinnedPages())

    result.current.pin('/page1', 'Page 1')
    result.current.pin('/page2', 'Page 2')
    result.current.unpin('/page1')

    const stored = JSON.parse(
      localStorage.getItem(PINNED_PAGES_KEY) || '[]',
    ) as Array<{ path: string; title: string }>
    expect(stored).toEqual([{ path: '/page2', title: 'Page 2' }])
  })

  it('checks if a page is pinned', () => {
    const { result } = renderHook(() => usePinnedPages())

    result.current.pin('/page1', 'Page 1')

    expect(result.current.isPinned('/page1')).toBe(true)
    expect(result.current.isPinned('/page2')).toBe(false)
  })

  it('toggles a page pin state', () => {
    const { result } = renderHook(() => usePinnedPages())

    result.current.toggle('/page1', 'Page 1')
    expect(result.current.isPinned('/page1')).toBe(true)

    result.current.toggle('/page1', 'Page 1')
    expect(result.current.isPinned('/page1')).toBe(false)
  })

  it('prevents duplicate pins', () => {
    const { result } = renderHook(() => usePinnedPages())

    result.current.pin('/page1', 'Page 1')
    result.current.pin('/page1', 'Page 1')

    const stored = JSON.parse(
      localStorage.getItem(PINNED_PAGES_KEY) || '[]',
    ) as Array<{ path: string; title: string }>
    expect(stored).toHaveLength(1)
  })
})
