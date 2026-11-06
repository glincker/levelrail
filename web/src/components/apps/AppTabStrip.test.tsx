import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AppTabStrip } from './AppTabStrip'

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
vi.mock('./useAppTabs', () => ({
  useAppTabs: () => ({
    groups: [
      {
        id: 'deploy',
        items: [
          { id: 'a', slug: 'deploys', label: 'Deploys', to: '/d' },
          { id: 'b', slug: 'logs', label: 'Logs', to: '/l' },
        ],
      },
    ],
    activeGroup: 'deploy',
    activeSlug: 'deploys',
    isStatic: false,
    hiddenCount: 0,
    showAll: false,
    setShowAll: vi.fn(),
  }),
}))

describe('AppTabStrip', () => {
  it('scrolls the tabs inside their own container with edge fades', () => {
    const { container } = render(<AppTabStrip name="web" />)
    const nav = container.querySelector('nav')
    expect(nav).toHaveClass('overflow-x-auto', 'min-w-0')
    const scroller = container.querySelector('[data-slot="app-tab-scroller"]')
    expect(scroller).toHaveClass('min-w-0', 'flex-1')
    const fades = scroller?.querySelectorAll('[data-edge]')
    expect(fades).toHaveLength(2)
    fades?.forEach((el) => {
      expect(el).toHaveAttribute('aria-hidden', 'true')
      expect(el).toHaveClass('motion-reduce:transition-none')
    })
    expect(screen.getByRole('link', { name: 'Deploys' })).toHaveAttribute(
      'aria-current',
      'page',
    )
  })
})
