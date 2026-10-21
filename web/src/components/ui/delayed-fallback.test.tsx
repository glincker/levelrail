import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../../test/commonI18n'
import { DelayedFallback } from './delayed-fallback'

describe('DelayedFallback', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders nothing at first, then the page skeleton after the delay', () => {
    render(
      <CommonI18n>
        <DelayedFallback />
      </CommonI18n>,
    )
    expect(screen.queryByRole('status')).toBeNull()
    act(() => {
      vi.advanceTimersByTime(200)
    })
    expect(screen.getByRole('status')).toBeTruthy()
  })

  it('never shows the skeleton if it unmounts before the delay', () => {
    const { unmount } = render(
      <CommonI18n>
        <DelayedFallback />
      </CommonI18n>,
    )
    unmount()
    act(() => {
      vi.advanceTimersByTime(500)
    })
    expect(screen.queryByRole('status')).toBeNull()
  })
})
