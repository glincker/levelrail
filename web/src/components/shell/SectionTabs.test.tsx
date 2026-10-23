import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SectionTabs } from './SectionTabs'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children?: ReactNode
    to?: string
  } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))

const items = [
  { to: '/deployments', label: 'Deploys', active: false },
  { to: '/deploy-timeline', label: 'Timeline', active: true },
  { to: '/builds', label: 'Builds', active: false },
] as const

describe('SectionTabs', () => {
  it('renders a labelled nav of real links without tab roles', () => {
    render(<SectionTabs ariaLabel="Deployment sections" items={items} />)
    const nav = screen.getByRole('navigation', { name: 'Deployment sections' })
    expect(nav).toBeInTheDocument()
    expect(screen.queryByRole('tab')).toBeNull()
    expect(
      screen.getAllByRole('link').map((l) => l.getAttribute('href')),
    ).toEqual(['/deployments', '/deploy-timeline', '/builds'])
  })

  it('marks only the active tab as the current page', () => {
    render(<SectionTabs ariaLabel="Sections" items={items} />)
    expect(screen.getByRole('link', { name: 'Timeline' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(screen.getByRole('link', { name: 'Deploys' })).not.toHaveAttribute(
      'aria-current',
    )
    expect(screen.getByRole('link', { name: 'Builds' })).not.toHaveAttribute(
      'aria-current',
    )
  })

  it('scrolls horizontally on narrow screens', () => {
    render(<SectionTabs ariaLabel="Sections" items={items} />)
    expect(screen.getByRole('navigation')).toHaveClass('overflow-x-auto')
  })
})
