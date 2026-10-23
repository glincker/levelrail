import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DRAFT_SAVE_DEBOUNCE_MS, draftStorageKey, useDraft } from './useDraft'

interface Form {
  name: string
  password: string
  nested: { apiToken: string; note: string }
  custom: string
}

const INITIAL: Form = {
  name: '',
  password: '',
  nested: { apiToken: '', note: '' },
  custom: '',
}

function stored(key: string): Record<string, unknown> | null {
  const raw = window.localStorage.getItem(draftStorageKey(key))
  return raw ? (JSON.parse(raw) as Record<string, unknown>) : null
}

function seed(key: string, patch: Record<string, unknown>) {
  window.localStorage.setItem(
    draftStorageKey(key),
    JSON.stringify({
      v: 1,
      savedAt: Date.now(),
      value: { ...INITIAL, name: 'saved' },
      ...patch,
    }),
  )
}

beforeEach(() => {
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllEnvs()
  vi.restoreAllMocks()
})

describe('useDraft', () => {
  it('autosaves after the debounce and restores on a fresh mount', () => {
    const first = renderHook(() => useDraft('t1', INITIAL))
    act(() => first.result.current.update({ name: 'hello' }))
    expect(stored('t1')).toBeNull()
    act(() => {
      vi.advanceTimersByTime(DRAFT_SAVE_DEBOUNCE_MS + 1)
    })
    expect(stored('t1')?.value).toMatchObject({ name: 'hello' })
    first.unmount()

    const second = renderHook(() => useDraft('t1', INITIAL))
    expect(second.result.current.hasDraft).toBe(true)
    expect(second.result.current.value.name).toBe('')
    act(() => {
      second.result.current.restore()
    })
    expect(second.result.current.value.name).toBe('hello')
    expect(second.result.current.hasDraft).toBe(false)
  })

  it('drops a draft older than the TTL', () => {
    vi.stubEnv('VITE_DRAFT_TTL_HOURS', '1')
    seed('t2', { savedAt: Date.now() - 2 * 3_600_000 })
    const { result } = renderHook(() => useDraft('t2', INITIAL))
    expect(result.current.hasDraft).toBe(false)
    expect(stored('t2')).toBeNull()
  })

  it('drops a draft saved under another schema version', () => {
    seed('t3', { v: 1 })
    const { result } = renderHook(() => useDraft('t3', INITIAL, { version: 2 }))
    expect(result.current.hasDraft).toBe(false)
    expect(stored('t3')).toBeNull()
  })

  it('never stores secret-looking or omitted fields', () => {
    const { result } = renderHook(() =>
      useDraft('t4', INITIAL, { omit: ['custom'] }),
    )
    act(() =>
      result.current.update({
        name: 'n',
        password: 'hunter2',
        custom: 'private',
        nested: { apiToken: 'tok', note: 'ok' },
      }),
    )
    act(() => {
      vi.advanceTimersByTime(DRAFT_SAVE_DEBOUNCE_MS + 1)
    })
    const raw = window.localStorage.getItem(draftStorageKey('t4')) ?? ''
    expect(raw).not.toMatch(/hunter2|private|tok"/)
    expect(stored('t4')?.value).toEqual({ name: 'n', nested: { note: 'ok' } })
  })

  it('works when storage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const { result } = renderHook(() => useDraft('t5', INITIAL))
    expect(result.current.hasDraft).toBe(false)
    act(() => result.current.update({ name: 'still works' }))
    act(() => {
      vi.advanceTimersByTime(DRAFT_SAVE_DEBOUNCE_MS + 1)
    })
    expect(result.current.value.name).toBe('still works')
    act(() => result.current.clearOnSuccess())
  })

  it('discard and clearOnSuccess remove the stored draft', () => {
    const { result } = renderHook(() => useDraft('t6', INITIAL))
    act(() => result.current.update({ name: 'x' }))
    act(() => {
      vi.advanceTimersByTime(DRAFT_SAVE_DEBOUNCE_MS + 1)
    })
    expect(stored('t6')).not.toBeNull()
    act(() => result.current.clearOnSuccess())
    expect(stored('t6')).toBeNull()

    seed('t6', {})
    const again = renderHook(() => useDraft('t6', INITIAL))
    act(() => again.result.current.discard())
    expect(stored('t6')).toBeNull()
    expect(again.result.current.value).toEqual(INITIAL)
  })
})
