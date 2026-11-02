import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SaveStatus } from './save-status'

afterEach(() => vi.useRealTimers())

// i18n is not initialised in unit tests, so t() returns the key.
describe('SaveStatus', () => {
  it('keeps an empty polite live region when idle', () => {
    render(<SaveStatus state="idle" />)
    const el = screen.getByRole('status')
    expect(el.getAttribute('aria-live')).toBe('polite')
    expect(el.textContent).toBe('')
  })
  it.each([
    ['saving', 'saveStatus.saving'],
    ['saved', 'saveStatus.saved'],
    ['error', 'saveStatus.error'],
  ] as const)('shows %s', async (state, text) => {
    render(<SaveStatus state={state} />)
    expect((await screen.findByRole('status')).textContent).toContain(text)
  })
  it('clears Saved after the delay', async () => {
    vi.useFakeTimers()
    render(<SaveStatus state="saved" clearAfterMs={100} />)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(screen.getByRole('status').textContent).toContain('saveStatus.saved')
    await act(async () => {
      await vi.advanceTimersByTimeAsync(150)
    })
    expect(screen.getByRole('status').textContent).toBe('')
  })
})
