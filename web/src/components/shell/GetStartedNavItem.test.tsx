import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { GetStartedNavItem } from './GetStartedNavItem'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
  useRouterState: () => '/',
}))
vi.mock('@/hooks/useDemo', () => ({ useIsDemo: () => false }))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: { done?: number; total?: number }) =>
      key === 'chip.steps' ? `${o?.done} of ${o?.total} steps done` : key,
  }),
}))
vi.mock('@/components/ui/sidebar', () => ({
  SidebarMenu: ({ children }: { children?: React.ReactNode }) => (
    <ul>{children}</ul>
  ),
  SidebarMenuItem: ({ children }: { children?: React.ReactNode }) => (
    <li>{children}</li>
  ),
  SidebarMenuButton: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))
vi.mock('../../queries/setupGuide', () => ({
  useSetupGuide: () => ({
    data: {
      steps: [],
      done: 3,
      total: 6,
      ready_percent: 50,
      complete: false,
      hidden: false,
    },
  }),
}))

describe('GetStartedNavItem', () => {
  it('shows a ring with the step count', () => {
    render(<GetStartedNavItem />)
    const ring = screen.getByRole('progressbar', { name: '3 of 6 steps done' })
    expect(ring).toHaveAttribute('aria-valuenow', '50')
    expect(screen.getByText('3/6')).toBeInTheDocument()
  })
})
