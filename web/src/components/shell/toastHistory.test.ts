import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  TOAST_HISTORY_STORAGE_KEY,
  clearToastHistory,
  recordToastHistory,
  resetToastHistoryForTests,
  useToastHistory,
} from './toastHistory'

afterEach(() => {
  window.localStorage.clear()
  resetToastHistoryForTests()
  vi.restoreAllMocks()
})

describe('recordToastHistory', () => {
  it('records newest first', () => {
    recordToastHistory({ title: 'first' })
    recordToastHistory({ title: 'second' })
    const stored = JSON.parse(
      window.localStorage.getItem(TOAST_HISTORY_STORAGE_KEY) ?? '[]',
    ) as { title: string }[]
    expect(stored.map((e) => e.title)).toEqual(['second', 'first'])
  })

  it('assigns unique ids and a timestamp', () => {
    recordToastHistory({ title: 'a' })
    recordToastHistory({ title: 'b' })
    const stored = JSON.parse(
      window.localStorage.getItem(TOAST_HISTORY_STORAGE_KEY) ?? '[]',
    ) as { id: string; timestamp: number }[]
    expect(stored[0]?.id).not.toBe(stored[1]?.id)
    expect(typeof stored[0]?.timestamp).toBe('number')
  })

  it('keeps an optional description', () => {
    recordToastHistory({ title: 'a', description: 'details' })
    const stored = JSON.parse(
      window.localStorage.getItem(TOAST_HISTORY_STORAGE_KEY) ?? '[]',
    ) as { description?: string }[]
    expect(stored[0]?.description).toBe('details')
  })

  it('caps the list at 50 entries', () => {
    for (let i = 0; i < 55; i += 1) {
      recordToastHistory({ title: `toast-${String(i)}` })
    }
    const stored = JSON.parse(
      window.localStorage.getItem(TOAST_HISTORY_STORAGE_KEY) ?? '[]',
    ) as { title: string }[]
    expect(stored).toHaveLength(50)
    expect(stored[0]?.title).toBe('toast-54')
  })
})

describe('clearToastHistory', () => {
  it('empties the stored list', () => {
    recordToastHistory({ title: 'a' })
    clearToastHistory()
    expect(window.localStorage.getItem(TOAST_HISTORY_STORAGE_KEY)).toBe('[]')
  })
})

describe('persistence round-trip', () => {
  it('ignores corrupt stored data', () => {
    window.localStorage.setItem(TOAST_HISTORY_STORAGE_KEY, '{not json')
    const { result } = renderHook(() => useToastHistory())
    expect(result.current).toEqual([])
  })

  it('loads previously stored entries', () => {
    recordToastHistory({ title: 'persisted' })
    resetToastHistoryForTests()
    const { result } = renderHook(() => useToastHistory())
    expect(result.current.map((e) => e.title)).toEqual(['persisted'])
  })
})

describe('useToastHistory', () => {
  it('updates reactively when a new toast is recorded', () => {
    const { result } = renderHook(() => useToastHistory())
    expect(result.current).toEqual([])
    act(() => {
      recordToastHistory({ title: 'new error' })
    })
    expect(result.current.map((e) => e.title)).toEqual(['new error'])
  })

  it('works when storage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const { result } = renderHook(() => useToastHistory())
    expect(result.current).toEqual([])
    act(() => {
      recordToastHistory({ title: 'x' })
    })
    expect(result.current.map((e) => e.title)).toEqual(['x'])
  })
})
