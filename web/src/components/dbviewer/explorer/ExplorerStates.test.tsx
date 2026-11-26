import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ExplorerLoading } from './ExplorerStates'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/x">{children}</a>
  ),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

describe('ExplorerLoading', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('shows a skeleton first, then a stuck state with retry and links', () => {
    render(<ExplorerLoading databaseName="pg" onRetry={() => undefined} />)
    expect(screen.getByRole('status')).toBeTruthy()
    act(() => {
      vi.advanceTimersByTime(20001)
    })
    expect(screen.getByRole('alert')).toBeTruthy()
    expect(screen.getByText('retry')).toBeTruthy()
    expect(screen.getByText('logs')).toBeTruthy()
    expect(screen.getByText('overview')).toBeTruthy()
  })
})
