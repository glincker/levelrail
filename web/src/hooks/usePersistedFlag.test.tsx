import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { usePersistedFlag } from './usePersistedFlag'

describe('usePersistedFlag', () => {
  beforeEach(() => window.localStorage.clear())

  it('remembers the toggled value across mounts', () => {
    const first = renderHook(() => usePersistedFlag('fold-test'))
    expect(first.result.current[0]).toBe(false)
    act(() => first.result.current[1]((prev) => !prev))
    expect(first.result.current[0]).toBe(true)
    first.unmount()
    const second = renderHook(() => usePersistedFlag('fold-test'))
    expect(second.result.current[0]).toBe(true)
  })
})
