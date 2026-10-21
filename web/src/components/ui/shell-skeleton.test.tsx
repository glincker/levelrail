import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../../test/commonI18n'
import { PageSpinner } from './page-spinner'
import { RootFallback, ShellSkeleton } from './shell-skeleton'

const stored = vi.hoisted(() => ({ username: null as string | null }))
vi.mock('../../lib/authStore', () => ({
  getStoredUsername: () => stored.username,
}))

function renderIn(node: React.ReactNode) {
  return render(<CommonI18n>{node}</CommonI18n>)
}

describe('shell skeletons', () => {
  beforeEach(() => {
    stored.username = null
  })

  it('draws the sidebar, top bar and centered body for a signed-in session', () => {
    stored.username = 'dev'
    const { container } = renderIn(<RootFallback />)
    expect(container.querySelector('main')).not.toBeNull()
    expect(container.querySelector('.md\\:block')).not.toBeNull()
    expect(screen.getAllByRole('status')).toHaveLength(1)
  })

  it('draws a centered card, not a shell, when signed out', () => {
    const { container } = renderIn(<RootFallback />)
    expect(container.querySelector('main')).toBeNull()
    expect(screen.getAllByRole('status')).toHaveLength(1)
  })

  it('announces loading once with the localized label', () => {
    renderIn(<ShellSkeleton />)
    expect(screen.getByRole('status').getAttribute('aria-label')).toBeTruthy()
    expect(screen.getByRole('status').getAttribute('aria-busy')).toBe('true')
  })

  it('keeps the in-shell page fallback to the body only', () => {
    const { container } = renderIn(<PageSpinner />)
    expect(container.querySelector('main')).toBeNull()
    expect(screen.getAllByRole('status')).toHaveLength(1)
  })
})
