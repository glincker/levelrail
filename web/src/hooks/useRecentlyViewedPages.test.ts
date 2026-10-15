import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useRecentlyViewedPages } from './useRecentlyViewedPages'

const RECENTLY_VIEWED_KEY = 'levelrail:recently-viewed'

describe('useRecentlyViewedPages', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('returns empty list initially', () => {
    const { result } = renderHook(() =>
      useRecentlyViewedPages('/getting-started', 'Getting Started'),
    )
    expect(result.current).toEqual([])
  })

  it('tracks recently viewed pages', () => {
    renderHook(({ path, title }) => useRecentlyViewedPages(path, title), {
      initialProps: { path: '/getting-started', title: 'Getting Started' },
    })

    const stored = JSON.parse(
      localStorage.getItem(RECENTLY_VIEWED_KEY) || '[]',
    ) as Array<{ path: string; title: string }>
    expect(stored).toHaveLength(1)
    expect(stored[0]?.path).toBe('/getting-started')
    expect(stored[0]?.title).toBe('Getting Started')
  })

  it('keeps recent pages in order (newest first)', () => {
    const { rerender } = renderHook(
      ({ path, title }) => useRecentlyViewedPages(path, title),
      {
        initialProps: { path: '/getting-started', title: 'Getting Started' },
      },
    )

    rerender({ path: '/deployment', title: 'Deployment' })

    const stored = JSON.parse(
      localStorage.getItem(RECENTLY_VIEWED_KEY) || '[]',
    ) as Array<{ path: string }>
    expect(stored[0]?.path).toBe('/deployment')
    expect(stored[1]?.path).toBe('/getting-started')
  })

  it('limits to 5 most recent pages', () => {
    const pages = [
      { path: '/page1', title: 'Page 1' },
      { path: '/page2', title: 'Page 2' },
      { path: '/page3', title: 'Page 3' },
      { path: '/page4', title: 'Page 4' },
      { path: '/page5', title: 'Page 5' },
      { path: '/page6', title: 'Page 6' },
    ]

    const hook = renderHook(
      ({ path, title }) => useRecentlyViewedPages(path, title),
      {
        initialProps: pages[0],
      },
    )

    for (let i = 1; i < pages.length; i++) {
      hook.rerender(pages[i])
    }

    const stored = JSON.parse(
      localStorage.getItem(RECENTLY_VIEWED_KEY) || '[]',
    ) as Array<{ path: string }>
    expect(stored).toHaveLength(5)
    expect(stored[0]?.path).toBe('/page6')
    expect(stored[4]?.path).toBe('/page2')
  })

  it('deduplicates re-visited pages (moves them to top)', () => {
    const { rerender } = renderHook(
      ({ path, title }) => useRecentlyViewedPages(path, title),
      {
        initialProps: { path: '/page1', title: 'Page 1' },
      },
    )

    rerender({ path: '/page2', title: 'Page 2' })
    rerender({ path: '/page1', title: 'Page 1' })

    const stored = JSON.parse(
      localStorage.getItem(RECENTLY_VIEWED_KEY) || '[]',
    ) as Array<{ path: string }>
    expect(stored).toHaveLength(2)
    expect(stored[0]?.path).toBe('/page1')
    expect(stored[1]?.path).toBe('/page2')
  })

  it('ignores root path', () => {
    renderHook(({ path, title }) => useRecentlyViewedPages(path, title), {
      initialProps: { path: '/', title: 'Home' },
    })

    const stored = JSON.parse(
      localStorage.getItem(RECENTLY_VIEWED_KEY) || '[]',
    ) as Array<{ path: string }>
    expect(stored).toHaveLength(0)
  })
})
